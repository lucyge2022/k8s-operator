#!/usr/bin/env bash

SCRIPT_DIR="$(dirname -- "$0")"
EXPECTED_TEMPLATES_DIR="${SCRIPT_DIR}/expectedTemplates"
TARGET_DIR="${SCRIPT_DIR}/resultTemplates"

function main {
  mkdir -p "${TARGET_DIR}"

  local return_code=0
  verifyMasterTemplate
  return_code=$((return_code || $?))
  verifyWorkerTemplate
  return_code=$((return_code || $?))
  verifyFuseTemplate
  return_code=$((return_code || $?))
  verifyProxyTemplate
  return_code=$((return_code || $?))
  verifyConfTemplate
  return_code=$((return_code || $?))
  verifyCsiTemplate
  exit $((return_code || $?))
}

function verifyMasterTemplate {
  mkdir -p "${TARGET_DIR}"/master
  local statefulset_relative_path="master/statefulset.yaml"
  helm template --name-template dummy "${SCRIPT_DIR}"/../../deploy/charts/alluxio --show-only templates/"${statefulset_relative_path}" -f "${SCRIPT_DIR}"/config_test.yaml --debug > "${TARGET_DIR}/${statefulset_relative_path}"
  cmp --silent "${TARGET_DIR}/${statefulset_relative_path}" "${EXPECTED_TEMPLATES_DIR}/${statefulset_relative_path}"
  if [[ $? -ne 0 ]]; then
    echo "Alluxio Master StatefulSet template is not rendered as expected."
    return 1
  fi
  local service_relative_path="master/service.yaml"
  helm template --name-template dummy "${SCRIPT_DIR}"/../../deploy/charts/alluxio --show-only templates/"${service_relative_path}" -f "${SCRIPT_DIR}"/config_test.yaml --debug > "${TARGET_DIR}/${service_relative_path}"
  cmp --silent "${TARGET_DIR}/${service_relative_path}" "${EXPECTED_TEMPLATES_DIR}/${service_relative_path}"
  if [[ $? -ne 0 ]]; then
    echo "Alluxio Master service template is not rendered as expected."
    return 1
  fi
}

function verifyWorkerTemplate {
  mkdir -p "${TARGET_DIR}"/worker
  local deployment_relative_path="worker/deployment.yaml"
  helm template --name-template dummy "${SCRIPT_DIR}"/../../deploy/charts/alluxio --show-only templates/"${deployment_relative_path}" -f "${SCRIPT_DIR}"/config_test.yaml --debug > "${TARGET_DIR}/${deployment_relative_path}"
  cmp --silent "${TARGET_DIR}/${deployment_relative_path}" "${EXPECTED_TEMPLATES_DIR}/${deployment_relative_path}"
  if [[ $? -ne 0 ]]; then
    echo "Alluxio Worker Deployment template is not rendered as expected."
    return 1
  fi
  local pageStorePvc_relative_path="worker/pageStore-pvc.yaml"
  helm template --name-template dummy "${SCRIPT_DIR}"/../../deploy/charts/alluxio --show-only templates/"${pageStorePvc_relative_path}" -f "${SCRIPT_DIR}"/config_test.yaml --debug > "${TARGET_DIR}/${pageStorePvc_relative_path}"
  cmp --silent "${TARGET_DIR}/${pageStorePvc_relative_path}" "${EXPECTED_TEMPLATES_DIR}/${pageStorePvc_relative_path}"
  if [[ $? -ne 0 ]]; then
    echo "Alluxio Worker page store PVC template is not rendered as expected."
    return 1
  fi
  local metastorePvc_relative_path="worker/metastore-pvc.yaml"
  helm template --name-template dummy "${SCRIPT_DIR}"/../../deploy/charts/alluxio --show-only templates/"${metastorePvc_relative_path}" -f "${SCRIPT_DIR}"/config_test.yaml --debug > "${TARGET_DIR}/${metastorePvc_relative_path}"
  cmp --silent "${TARGET_DIR}/${metastorePvc_relative_path}" "${EXPECTED_TEMPLATES_DIR}/${metastorePvc_relative_path}"
  if [[ $? -ne 0 ]]; then
    echo "Alluxio Worker metastore PVC template is not rendered as expected."
    return 1
  fi
}

function verifyFuseTemplate {
  mkdir -p "${TARGET_DIR}"/fuse
  local daemonset_relative_path="fuse/daemonset.yaml"
  helm template --name-template dummy "${SCRIPT_DIR}"/../../deploy/charts/alluxio --show-only templates/"${daemonset_relative_path}" -f "${SCRIPT_DIR}"/config_test.yaml --debug > "${TARGET_DIR}/${daemonset_relative_path}"
  cmp --silent "${TARGET_DIR}/${daemonset_relative_path}" "${EXPECTED_TEMPLATES_DIR}/${daemonset_relative_path}"
  if [[ $? -ne 0 ]]; then
    echo "Alluxio Fuse DaemonSet template is not rendered as expected."
    return 1
  fi
}

function verifyProxyTemplate {
  mkdir -p "${TARGET_DIR}"/proxy
  local daemonset_relative_path="proxy/daemonset.yaml"
  helm template --name-template dummy "${SCRIPT_DIR}"/../../deploy/charts/alluxio --show-only templates/"${daemonset_relative_path}" -f "${SCRIPT_DIR}"/config_test.yaml --debug > "${TARGET_DIR}/${daemonset_relative_path}"
  cmp --silent "${TARGET_DIR}/${daemonset_relative_path}" "${EXPECTED_TEMPLATES_DIR}/${daemonset_relative_path}"
  if [[ $? -ne 0 ]]; then
    echo "Alluxio Proxy DaemonSet template is not rendered as expected."
    return 1
  fi
}

function verifyConfTemplate {
  mkdir -p "${TARGET_DIR}"/conf
  local configmap_relative_path="conf/configmap.yaml"
  helm template --name-template dummy "${SCRIPT_DIR}"/../../deploy/charts/alluxio --show-only templates/"${configmap_relative_path}" -f "${SCRIPT_DIR}"/config_test.yaml --debug > "${TARGET_DIR}/${configmap_relative_path}"
  cmp --silent "${TARGET_DIR}/${configmap_relative_path}" "${EXPECTED_TEMPLATES_DIR}/${configmap_relative_path}"
  if [[ $? -ne 0 ]]; then
    echo "Alluxio Conf dir configmap template is not rendered as expected."
    return 1
  fi
}

function verifyCsiTemplate {
  mkdir -p "${TARGET_DIR}"/csi
  local fuse_configmap_relative_path="csi/csi-fuse.yaml"
  helm template --name-template dummy "${SCRIPT_DIR}"/../../deploy/charts/alluxio --show-only templates/"${fuse_configmap_relative_path}" -f "${SCRIPT_DIR}"/config_test.yaml --debug > "${TARGET_DIR}/${fuse_configmap_relative_path}"
  cmp --silent "${TARGET_DIR}/${fuse_configmap_relative_path}" "${EXPECTED_TEMPLATES_DIR}/${fuse_configmap_relative_path}"
  if [[ $? -ne 0 ]]; then
    echo "Alluxio csi Fuse configmap template is not rendered as expected."
    return 1
  fi
  local pvc_relative_path="csi/pvc.yaml"
  helm template --name-template dummy "${SCRIPT_DIR}"/../../deploy/charts/alluxio --show-only templates/"${pvc_relative_path}" -f "${SCRIPT_DIR}"/config_test.yaml --debug > "${TARGET_DIR}/${pvc_relative_path}"
  cmp --silent "${TARGET_DIR}/${pvc_relative_path}" "${EXPECTED_TEMPLATES_DIR}/${pvc_relative_path}"
  if [[ $? -ne 0 ]]; then
    echo "Alluxio csi pvc template is not rendered as expected."
    return 1
  fi
  local storageClass_relative_path="csi/storageClass.yaml"
  helm template --name-template dummy "${SCRIPT_DIR}"/../../deploy/charts/alluxio --show-only templates/"${storageClass_relative_path}" -f "${SCRIPT_DIR}"/config_test.yaml --debug > "${TARGET_DIR}/${storageClass_relative_path}"
  cmp --silent "${TARGET_DIR}/${storageClass_relative_path}" "${EXPECTED_TEMPLATES_DIR}/${storageClass_relative_path}"
  if [[ $? -ne 0 ]]; then
    echo "Alluxio csi storageClass template is not rendered as expected."
    return 1
  fi
}

main