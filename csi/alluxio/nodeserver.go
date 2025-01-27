/*
 * The Alluxio Open Foundation licenses this work under the Apache License, version 2.0
 * (the "License"). You may not use this work except in compliance with the License, which is
 * available at www.apache.org/licenses/LICENSE-2.0
 *
 * This software is distributed on an "AS IS" basis, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND,
 * either express or implied, as more fully set forth in the License.
 *
 * See the NOTICE file distributed with this work for information regarding copyright ownership.
 */

package alluxio

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/glog"
	csicommon "github.com/kubernetes-csi/drivers/pkg/csi-common"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/mount"
)

type nodeServer struct {
	client kubernetes.Clientset
	*csicommon.DefaultNodeServer
	nodeId string
	mutex  sync.Mutex
}

/*
 * When there is no app pod using the pv, the first app pod using the pv would trigger NodeStageVolume().
 * Only after a successful return, NodePublishVolume() is called.
 * When a pv is already in use and a new app pod uses it as its volume, it would only trigger NodePublishVolume()
 *
 * NodeUnpublishVolume() and NodeUnstageVolume() are the opposites of NodePublishVolume() and NodeStageVolume()
 * When a pv would still be using by other pods after an app pod terminated, only NodeUnpublishVolume() is called.
 * When a pv would not be in use after an app pod terminated, NodeUnpublishVolume() is called. Only after a successful
 * return, NodeUnstageVolume() is called.
 *
 * For more detailed CSI doc, refer to https://github.com/container-storage-interface/spec/blob/master/spec.md
 */

func (ns *nodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	targetPath := req.GetTargetPath()
	stagingPath := req.GetStagingTargetPath()

	notMnt, err := ensureMountPoint(targetPath)
	if err != nil {
		glog.V(3).Infof("Error checking mount point: %+v.", err)
		return nil, status.Error(codes.Internal, err.Error())
	}
	if !notMnt {
		glog.V(4).Infoln("target path is already mounted")
		return &csi.NodePublishVolumeResponse{}, nil
	}

	args := []string{"--bind", stagingPath, targetPath}
	command := exec.Command("mount", args...)
	_, err = command.CombinedOutput()
	if err != nil {
		if os.IsPermission(err) {
			glog.V(3).Infof("Permission denied. Failed to run mount bind command. %+v", err)
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
		if strings.Contains(err.Error(), "invalid argument") {
			glog.V(3).Infof("Invalid argument for mount bind command %v. %+v", command, err)
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		glog.V(3).Infof("Error running command `%v`: %+v", command, err)
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &csi.NodePublishVolumeResponse{}, nil
}

func (ns *nodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	targetPath := req.GetTargetPath()

	err := mount.CleanupMountPoint(targetPath, mount.New(""), false)
	if err != nil {
		glog.V(3).Infof("Error cleaning up mount point: %+v", err)
	} else {
		glog.V(4).Infof("Succeed in unmounting %s", targetPath)
	}
	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (ns *nodeServer) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	ns.mutex.Lock()
	defer ns.mutex.Unlock()
	fusePod, err := getAndCompleteFusePodObj(ns, req)
	if err != nil {
		glog.V(3).Infof("Error getting or completing the CSI Fuse pod object. %+v", err)
		return nil, status.Error(codes.Internal, err.Error())
	}
	if err := ns.createFusePodIfNotExist(fusePod); err != nil {
		glog.V(3).Infof("Error creating CSI Fuse pod. %+v", err)
		return nil, status.Error(codes.Internal, err.Error())
	}
	if err := checkIfMountPointReady(req.GetStagingTargetPath()); err != nil {
		glog.V(3).Infof("Mount point is not ready, or error occurs. %+v", err)
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &csi.NodeStageVolumeResponse{}, nil
}

func getAndCompleteFusePodObj(ns *nodeServer, req *csi.NodeStageVolumeRequest) (*v1.Pod, error) {
	alluxioNamespacedName := types.NamespacedName{
		Namespace: req.GetVolumeContext()["alluxioClusterNamespace"],
		Name:      req.GetVolumeContext()["alluxioClusterName"],
	}
	csiFusePodObj, err := getFusePodObj(ns, alluxioNamespacedName)
	if err != nil {
		return nil, errors.Wrap(err, "Error getting Fuse pod object from template.")
	}

	// Append extra information to pod name for uniqueness but not exceed maximum
	csiFusePodObj.Name = getFusePodName(alluxioNamespacedName.Name, ns.nodeId, req.GetVolumeId())[:64]

	csiFusePodObj.Namespace = alluxioNamespacedName.Namespace

	// Set node name for scheduling
	csiFusePodObj.Spec.NodeName = ns.nodeId

	// Set fuse mount point
	csiFusePodObj.Spec.Containers[0].Args[2] = req.GetStagingTargetPath()

	// Set pre-stop command (umount) in pod lifecycle
	lifecycle := &v1.Lifecycle{
		PreStop: &v1.Handler{
			Exec: &v1.ExecAction{
				Command: []string{"/opt/alluxio/integration/fuse/bin/alluxio-fuse", "unmount", req.GetStagingTargetPath()},
			},
		},
	}
	csiFusePodObj.Spec.Containers[0].Lifecycle = lifecycle
	return csiFusePodObj, nil
}

func (ns *nodeServer) createFusePodIfNotExist(fusePod *v1.Pod) error {
	if _, err := ns.client.CoreV1().Pods(fusePod.Namespace).Create(fusePod); err != nil {
		if strings.Contains(err.Error(), "already exists") {
			glog.V(4).Infof("Fuse pod %s already exists. Skip creating pod.", fusePod.Name)
		} else {
			return errors.Wrap(err, "Error creating Fuse pod.")
		}
	}
	return nil
}

func checkIfMountPointReady(mountPoint string) error {
	command := exec.Command("sh", "-c", fmt.Sprintf("mount | grep %v | grep alluxio-fuse", mountPoint))
	_, err := command.Output()
	if err != nil {
		return errors.Wrap(err, "Mount point is not ready, or error occurs while checking.")
	}
	return nil
}

func (ns *nodeServer) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	pv, err := ns.client.CoreV1().PersistentVolumes().Get(req.VolumeId, metav1.GetOptions{})
	if err != nil {
		glog.V(3).Infof("Error getting PV with volume ID %v. %+v", req.VolumeId, err)
		return nil, status.Error(codes.Internal, err.Error())
	}
	storageClass, err := ns.client.StorageV1().StorageClasses().Get(pv.Spec.StorageClassName, metav1.GetOptions{})
	if err != nil {
		glog.V(3).Infof("Error getting StorageClass %v associated with the PV with volume ID %v. %+v", pv.Spec.StorageClassName, req.VolumeId, err)
		return nil, status.Error(codes.Internal, err.Error())
	}
	alluxioNamespacedName := types.NamespacedName{
		Namespace: storageClass.Parameters["alluxioClusterNamespace"],
		Name:      storageClass.Parameters["alluxioClusterName"],
	}
	podName := getFusePodName(alluxioNamespacedName.Name, ns.nodeId, req.GetVolumeId())
	if err := ns.client.CoreV1().Pods(alluxioNamespacedName.Namespace).Delete(podName, &metav1.DeleteOptions{}); err != nil {
		if strings.Contains(err.Error(), "not found") {
			// Pod not found. Try to clean up the mount point.
			command := exec.Command("umount", req.GetStagingTargetPath())
			_, err := command.CombinedOutput()
			if err != nil {
				glog.V(3).Infof("Error running command %v: %+v", command, err)
			}
			return &csi.NodeUnstageVolumeResponse{}, nil
		}
		glog.V(3).Infof("Error deleting pod with name %v. %+v.", podName, err)
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (ns *nodeServer) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (ns *nodeServer) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	return &csi.NodeGetCapabilitiesResponse{
		Capabilities: []*csi.NodeServiceCapability{
			{
				Type: &csi.NodeServiceCapability_Rpc{
					Rpc: &csi.NodeServiceCapability_RPC{
						Type: csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME,
					},
				},
			},
		},
	}, nil
}

func isCorruptedDir(dir string) bool {
	pathExists, pathErr := mount.PathExists(dir)
	glog.V(3).Infoln("isCorruptedDir(%s) returned with error: (%v, %v)\\n", dir, pathExists, pathErr)
	return pathErr != nil && mount.IsCorruptedMnt(pathErr)
}

func ensureMountPoint(targetPath string) (bool, error) {
	mounter := mount.New(targetPath)
	notMnt, err := mounter.IsLikelyNotMountPoint(targetPath)

	if err == nil {
		return notMnt, nil
	}
	if err != nil && os.IsNotExist(err) {
		if err := os.MkdirAll(targetPath, 0750); err != nil {
			return notMnt, errors.Wrapf(err, "Error creating dir %v", targetPath)
		}
		return true, nil
	}
	if isCorruptedDir(targetPath) {
		glog.V(3).Infoln("detected corrupted mount for targetPath [%s]", targetPath)
		if err := mounter.Unmount(targetPath); err != nil {
			return false, errors.Wrapf(err, "Filed to umount corrupted path %v", targetPath)
		}
		return true, nil
	}
	return notMnt, errors.Wrapf(err, "Failed to check if target path %v is a mount point.", targetPath)
}

func getFusePodObj(ns *nodeServer, alluxioNamespacedName types.NamespacedName) (*v1.Pod, error) {
	fuseConfigmapName := fmt.Sprintf("%v-csi-fuse-config", alluxioNamespacedName.Name)
	fuseConfigmap, err := ns.client.CoreV1().ConfigMaps(alluxioNamespacedName.Namespace).Get(fuseConfigmapName, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to find the configmap containing fuse pod object.")
	}
	csiFuseYaml := []byte(fuseConfigmap.Data["alluxio-csi-fuse.yaml"])
	csiFuseObj, grpVerKind, err := scheme.Codecs.UniversalDeserializer().Decode(csiFuseYaml, nil, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "Error decoding CSI Fuse yaml file to object.")
	}
	// Only support Fuse Pod
	if grpVerKind.Kind != "Pod" {
		return nil, errors.Wrapf(err, "CSI Fuse only supports Kind Pod. %v found.", grpVerKind.Kind)
	}
	return csiFuseObj.(*v1.Pod), nil
}

func getFusePodName(clusterName, nodeId, volumeId string) string {
	volumeIdParts := strings.Split(volumeId, "-")
	return strings.Join([]string{clusterName, nodeId, volumeIdParts[len(volumeIdParts)-1]}, "-")
}
