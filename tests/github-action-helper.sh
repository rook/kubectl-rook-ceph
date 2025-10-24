#!/usr/bin/env bash

set -xeEo pipefail

#############
# VARIABLES #
#############
: "${FUNCTION:=${1}}"

# source https://github.com/rook/rook
function find_extra_block_dev() {
  # shellcheck disable=SC2005 # redirect doesn't work with sudo, so use echo
  echo "$(sudo lsblk)" >/dev/stderr # print lsblk output to stderr for debugging in case of future errors
  # relevant lsblk --pairs example: (MOUNTPOINT identifies boot partition)(PKNAME is Parent dev ID)
  # NAME="sda15" SIZE="106M" TYPE="part" MOUNTPOINT="/boot/efi" PKNAME="sda"
  # NAME="sdb"   SIZE="75G"  TYPE="disk" MOUNTPOINT=""          PKNAME=""
  # NAME="sdb1"  SIZE="75G"  TYPE="part" MOUNTPOINT="/mnt"      PKNAME="sdb"
  boot_dev="$(sudo lsblk --noheading --list --output MOUNTPOINT,PKNAME | grep boot | awk '{print $2}')"
  echo "  == find_extra_block_dev(): boot_dev='$boot_dev'" >/dev/stderr # debug in case of future errors
  # --nodeps ignores partitions
  extra_dev="$(sudo lsblk --noheading --list --nodeps --output KNAME | grep -v loop | grep -v "$boot_dev" | head -1)"
  echo "  == find_extra_block_dev(): extra_dev='$extra_dev'" >/dev/stderr # debug in case of future errors
  echo "$extra_dev"                                                       # output of function
}

: "${BLOCK:=$(find_extra_block_dev)}"

# source https://github.com/rook/rook
use_local_disk() {
  BLOCK_DATA_PART="/dev/${BLOCK}1"
  sudo apt purge snapd -y
  sudo dmsetup version || true
  sudo swapoff --all --verbose
  sudo umount /mnt
  # search for the device since it keeps changing between sda and sdb
  sudo wipefs --all --force "$BLOCK_DATA_PART"
}

deploy_rook() {
  kubectl create -f https://raw.githubusercontent.com/rook/rook/master/deploy/examples/common.yaml
  kubectl create -f https://raw.githubusercontent.com/rook/rook/master/deploy/examples/crds.yaml
  kubectl create -f https://raw.githubusercontent.com/rook/rook/master/deploy/examples/operator.yaml
  kubectl create -f https://raw.githubusercontent.com/rook/rook/master/deploy/examples/csi-operator.yaml

  wait_for_operator_pod_to_be_ready_state rook-ceph
  curl https://raw.githubusercontent.com/rook/rook/master/deploy/examples/cluster-test.yaml -o cluster-test.yaml
  sed -i "s|#deviceFilter:|deviceFilter: ${BLOCK/\/dev\//}|g" cluster-test.yaml
  sed -i '0,/count: 1/ s/count: 1/count: 3/' cluster-test.yaml
  kubectl create -f cluster-test.yaml
  wait_for_pod_to_be_ready_state rook-ceph
  kubectl create -f https://raw.githubusercontent.com/rook/rook/master/deploy/examples/toolbox.yaml

  deploy_csi_driver_default_ns
}

deploy_rook_in_custom_namespace() {
  OPERATOR_NS=$1
  CLUSTER_NS=$2
  : "${OPERATOR_NS:=test-operator}"
  : "${CLUSTER_NS:=test-cluster}"

  kubectl create namespace test-operator # creating namespace manually because rook common.yaml create one namespace and here we need 2
  curl https://raw.githubusercontent.com/rook/rook/master/deploy/examples/common.yaml -o common.yaml
  deploy_with_custom_ns "$OPERATOR_NS" "$CLUSTER_NS" common.yaml
  kubectl create -f https://raw.githubusercontent.com/rook/rook/master/deploy/examples/crds.yaml

  curl -f https://raw.githubusercontent.com/rook/rook/master/deploy/examples/operator.yaml -o operator.yaml
  deploy_with_custom_ns "$OPERATOR_NS" "$CLUSTER_NS" operator.yaml

  curl -f https://raw.githubusercontent.com/rook/rook/master/deploy/examples/csi-operator.yaml -o csi-operator.yaml
  sed -i "s|namespace: rook-ceph|namespace: ${OPERATOR_NS}|g" csi-operator.yaml
  kubectl create -f csi-operator.yaml

  curl https://raw.githubusercontent.com/rook/rook/master/deploy/examples/cluster-test.yaml -o cluster-test.yaml
  sed -i "s|#deviceFilter:|deviceFilter: ${BLOCK/\/dev\//}|g" cluster-test.yaml
  sed -i '0,/count: 1/ s/count: 1/count: 3/' cluster-test.yaml
  deploy_with_custom_ns "$OPERATOR_NS" "$CLUSTER_NS" cluster-test.yaml
  wait_for_pod_to_be_ready_state $CLUSTER_NS

  curl -f https://raw.githubusercontent.com/rook/rook/master/deploy/examples/toolbox.yaml -o toolbox.yaml
  deploy_with_custom_ns "$OPERATOR_NS" "$CLUSTER_NS" toolbox.yaml

  deploy_csi_driver_custom_ns "$OPERATOR_NS" "$CLUSTER_NS"
}

create_sc_with_retain_policy() {
  export OPERATOR_NS=$1
  export CLUSTER_NS=$2

  curl https://raw.githubusercontent.com/rook/rook/master/deploy/examples/csi/cephfs/storageclass.yaml -o storageclass.yaml
  sed -i "s|name: rook-cephfs|name: rook-cephfs-retain|g" storageclass.yaml
  sed -i "s|reclaimPolicy: Delete|reclaimPolicy: Retain|g" storageclass.yaml
  sed -i "s|provisioner: rook-ceph.cephfs.csi.ceph.com |provisioner: ${OPERATOR_NS}.cephfs.csi.ceph.com |g" storageclass.yaml
  deploy_with_custom_ns $OPERATOR_NS $CLUSTER_NS storageclass.yaml
}

create_stale_subvolume() {
  curl https://raw.githubusercontent.com/rook/rook/master/deploy/examples/csi/cephfs/pvc.yaml -o pvc.yaml
  sed -i "s|name: cephfs-pvc|name: cephfs-pvc-retain|g" pvc.yaml
  sed -i "s|storageClassName: rook-cephfs|storageClassName: rook-cephfs-retain|g" pvc.yaml
  kubectl create -f pvc.yaml
  kubectl get pvc cephfs-pvc-retain
  wait_for_pvc_to_be_bound_state
  : "${PVNAME:=$(kubectl get pvc cephfs-pvc-retain -o=jsonpath='{.spec.volumeName}')}"
  kubectl get pvc cephfs-pvc-retain
  kubectl delete pvc cephfs-pvc-retain
  kubectl delete pv "$PVNAME"
}

deploy_with_custom_ns() {
  export OPERATOR_NS=$1
  export CLUSTER_NS=$2
  export MANIFEST=$3
  sed -i "s|rook-ceph # namespace:operator|${OPERATOR_NS} # namespace:operator|g" "${MANIFEST}"
  sed -i "s|rook-ceph # namespace:cluster|${CLUSTER_NS} # namespace:cluster|g" "${MANIFEST}"
  kubectl create -f "${MANIFEST}"
}

deploy_csi_driver_default_ns() {
  kubectl create -f https://raw.githubusercontent.com/rook/rook/master/deploy/examples/filesystem-test.yaml
  kubectl create -f https://raw.githubusercontent.com/rook/rook/master/deploy/examples/subvolumegroup.yaml
  kubectl create -f https://raw.githubusercontent.com/rook/rook/master/deploy/examples/csi/rbd/storageclass-test.yaml
  kubectl create -f https://raw.githubusercontent.com/rook/rook/master/deploy/examples/csi/rbd/pvc.yaml
  kubectl create -f https://raw.githubusercontent.com/rook/rook/master/deploy/examples/csi/cephfs/storageclass.yaml
  kubectl create -f https://raw.githubusercontent.com/rook/rook/master/deploy/examples/csi/cephfs/pvc.yaml
  deploy_csi_driver_rados_namespace
}

deploy_csi_driver_rados_namespace() {
  wait_for_cephblockpool_ready_state "rook-ceph" "replicapool" 60

  curl https://raw.githubusercontent.com/rook/rook/refs/heads/master/deploy/examples/pool-test.yaml -o pool_rados_ns.yaml
  sed -i "s|name: replicapool|name: blockpool-rados-ns |g" pool_rados_ns.yaml
  kubectl create -f pool_rados_ns.yaml
  wait_for_cephblockpool_ready_state "rook-ceph" "blockpool-rados-ns" 60

  curl https://raw.githubusercontent.com/rook/rook/refs/heads/master/deploy/examples/radosnamespace.yaml -o cephblockpoolradosnamespace_a.yaml
  sed -i "s|blockPoolName: replicapool|blockPoolName: blockpool-rados-ns |g" cephblockpoolradosnamespace_a.yaml
  kubectl create -f cephblockpoolradosnamespace_a.yaml
  wait_for_cephblockpoolradosnamespace_ready_state "rook-ceph" "namespace-a" 60

  # Restart the rook-ceph-operator pod to ensure it picks up new resources
  operator_pod=$(kubectl -n rook-ceph get pods -l app=rook-ceph-operator -o jsonpath='{.items[0].metadata.name}')
  if [ -n "$operator_pod" ]; then
    kubectl -n rook-ceph delete pod "$operator_pod"
    echo "rook-ceph-operator pod $operator_pod deleted. Waiting for it to restart..."
  else
    echo "rook-ceph-operator pod not found, skipping restart."
  fi
  wait_for_operator_pod_to_be_ready_state rook-ceph

  curl https://raw.githubusercontent.com/rook/rook/refs/heads/master/deploy/examples/radosnamespace.yaml -o cephblockpoolradosnamespace_b.yaml
  sed -i "s|blockPoolName: replicapool|blockPoolName: blockpool-rados-ns |g" cephblockpoolradosnamespace_b.yaml
  sed -i "s|name: namespace-a|name: namespace-b |g" cephblockpoolradosnamespace_b.yaml
  kubectl create -f cephblockpoolradosnamespace_b.yaml
  wait_for_cephblockpoolradosnamespace_ready_state "rook-ceph" "namespace-b" 60

  cluster_id=$(kubectl -n rook-ceph get cephblockpoolradosnamespace/namespace-a -o jsonpath='{.status.info.clusterID}')
  echo "cluster_id=${cluster_id}"

  curl https://raw.githubusercontent.com/rook/rook/refs/heads/master/deploy/examples/csi/rbd/storageclass-test.yaml -o storageclass-rados-namespace.yaml
  sed -i "s|clusterID: rook-ceph # namespace:cluster|clusterID: ${cluster_id} |g" storageclass-rados-namespace.yaml
  sed -i "s|name: rook-ceph-block|name: rook-ceph-block-rados-namespace |g" storageclass-rados-namespace.yaml
  sed -i "s|pool: replicapool|pool: blockpool-rados-ns |g" storageclass-rados-namespace.yaml
  kubectl apply -f storageclass-rados-namespace.yaml

  curl https://raw.githubusercontent.com/rook/rook/master/deploy/examples/csi/rbd/pvc.yaml -o pvc-rados-namespace.yaml
  sed -i "s|name: rbd-pvc|name: rbd-pvc-rados-namespace |g" pvc-rados-namespace.yaml
  sed -i "s|storageClassName: rook-ceph-block|storageClassName: rook-ceph-block-rados-namespace |g" pvc-rados-namespace.yaml
  kubectl create -f pvc-rados-namespace.yaml
}

deploy_csi_driver_custom_ns() {
  curl https://raw.githubusercontent.com/rook/rook/master/deploy/examples/csi/rbd/storageclass-test.yaml -o storageclass-rbd-test.yaml
  sed -i "s|provisioner: rook-ceph.rbd.csi.ceph.com |provisioner: test-operator.rbd.csi.ceph.com |g" storageclass-rbd-test.yaml
  deploy_with_custom_ns "$1" "$2" storageclass-rbd-test.yaml

  curl https://raw.githubusercontent.com/rook/rook/master/deploy/examples/csi/cephfs/storageclass.yaml -o storageclass-cephfs-test.yaml
  sed -i "s|provisioner: rook-ceph.cephfs.csi.ceph.com |provisioner: test-operator.cephfs.csi.ceph.com |g" storageclass-cephfs-test.yaml
  deploy_with_custom_ns "$1" "$2" storageclass-cephfs-test.yaml
  kubectl create -f https://raw.githubusercontent.com/rook/rook/master/deploy/examples/csi/rbd/pvc.yaml

  curl -f https://raw.githubusercontent.com/rook/rook/master/deploy/examples/filesystem-test.yaml -o filesystem.yaml
  deploy_with_custom_ns "$1" "$2" filesystem.yaml

  curl -f https://raw.githubusercontent.com/rook/rook/master/deploy/examples/subvolumegroup.yaml -o subvolumegroup.yaml
  deploy_with_custom_ns "$1" "$2" subvolumegroup.yaml
  kubectl create -f https://raw.githubusercontent.com/rook/rook/master/deploy/examples/csi/cephfs/pvc.yaml
}

wait_for_cephblockpoolradosnamespace_ready_state() {
  local cluster_ns=$1
  local namespace_name=$2
  local timeout_duration=${3:-10} # Default timeout to 10 seconds if not provided

  export cluster_ns namespace_name # Export variables for use in the subshell

  timeout "$timeout_duration" bash <<'EOF'
    set -x
    until [ "$(kubectl get CephBlockPoolRadosNamespace "$namespace_name" -n "$cluster_ns" -o jsonpath='{.status.phase}')" == "Ready" ]; do
      echo "Waiting for CephBlockPoolRadosNamespace '$namespace_name' to be in 'Ready' phase..."
      sleep 1
      kubectl get CephBlockPoolRadosNamespace -A
    done
    echo "CephBlockPoolRadosNamespace $namespace_name is in Ready phase!"
EOF

  timeout_command_exit_code
}

wait_for_cephblockpool_ready_state() {
  local cluster_ns=$1
  local blockpool_name=$2
  local timeout_duration=${3:-10} # Default timeout to 10 seconds if not provided

  export cluster_ns blockpool_name # Export variables for use in the subshell

  timeout "$timeout_duration" bash <<'EOF'
    set -x
    until [ "$(kubectl get CephBlockPool "$blockpool_name" -n "$cluster_ns" -o jsonpath='{.status.phase}')" == "Ready" ]; do
      echo "Waiting for CephBlockPool $blockpool_name to be in Ready phase"
      sleep 1
      kubectl get CephBlockPool -A
      kubectl describe CephBlockPool blockpool-rados-ns -n rook-ceph
      kubectl get CephBlockPool blockpool-rados-ns -n rook-ceph -o yaml
    done
    echo "CephBlockPool $blockpool_name is in Ready phase!"
EOF

  timeout_command_exit_code
}

wait_for_pvc_to_be_bound_state() {
  timeout 100 bash <<-'EOF'
    set -x
    until [ $(kubectl get pvc cephfs-pvc-retain -o jsonpath='{.status.phase}') == "Bound" ]; do
      echo "waiting for the pvc to be in bound state"
      sleep 1
    done
EOF
  timeout_command_exit_code
}

wait_for_pod_to_be_ready_state() {
  export cluster_ns=$1
  timeout 200 bash <<-'EOF'
    set -x
    until [ $(kubectl get pod -l app=rook-ceph-osd -n "${cluster_ns}" -o jsonpath='{.items[*].metadata.name}' -o custom-columns=READY:status.containerStatuses[*].ready | grep -c true) -eq 1 ]; do
      echo "waiting for the pods to be in ready state"
      sleep 1
    done
EOF
  timeout_command_exit_code
}

wait_for_operator_pod_to_be_ready_state() {
  export operator_ns=$1
  timeout 100 bash <<-'EOF'
    set -x
    until [ $(kubectl get pod -l app=rook-ceph-operator -n "${operator_ns}" -o jsonpath='{.items[*].metadata.name}' -o custom-columns=READY:status.containerStatuses[*].ready | grep -c true) -eq 1 ]; do
      echo "waiting for the operator to be in ready state"
      sleep 1
    done
    echo "rook-ceph-operator pod is in ready state!"
EOF
}

wait_for_three_mons() {
  export namespace=$1
  timeout 150 bash <<-'EOF'
    set -x
    until [ $(kubectl -n $namespace get deploy -l app=rook-ceph-mon,mon_canary!=true | grep rook-ceph-mon | wc -l | awk '{print $1}' ) -eq 3 ]; do
      echo "$(date) waiting for three mon deployments to exist"
      sleep 2
    done
EOF
  timeout_command_exit_code
}

wait_for_deployment_to_be_running() {
  deployment=$1
  namespace=$2
  echo "Waiting for the pod from deployment \"$deployment\" to be running"
  kubectl -n "$namespace" wait deployment "$deployment" --for condition=Available=True --timeout=90s
}

wait_for_ceph_cluster_to_be_ready() {
  export cluster_ns=$1
  timeout 300 bash <<-'EOF'
    set -x
    until [ $(kubectl -n "${cluster_ns}" get cephcluster my-cluster -o=jsonpath='{.status.phase}') == "Ready" ]; do
      echo "Waiting for the CephCluster my-cluster to be in the Ready state..."
      current_phase=$(kubectl -n "${cluster_ns}" get cephcluster my-cluster -o=jsonpath='{.status.phase}' 2>/dev/null || echo "not found")
      echo "Current CephCluster phase: $current_phase"
      kubectl get cephcluster -n "${cluster_ns}" -o wide 2>/dev/null || echo "CephCluster not accessible"
      sleep 2
    done
    echo "CephCluster my-cluster is now in Ready state!"
EOF
}

timeout_command_exit_code() {
  # timeout command return exit status 124 if command times out
  if [ $? -eq 124 ]; then
    echo "Timeout reached"
    exit 1
  fi
}

install_minikube_with_none_driver() {
  CRICTL_VERSION="v1.31.1"
  MINIKUBE_VERSION="v1.34.0"

  sudo apt update
  sudo apt install -y conntrack socat
  curl -LO https://storage.googleapis.com/minikube/releases/$MINIKUBE_VERSION/minikube_latest_amd64.deb
  sudo dpkg -i minikube_latest_amd64.deb
  rm -f minikube_latest_amd64.deb

  curl -LO https://github.com/Mirantis/cri-dockerd/releases/download/v0.3.15/cri-dockerd_0.3.15.3-0.ubuntu-focal_amd64.deb
  sudo dpkg -i cri-dockerd_0.3.15.3-0.ubuntu-focal_amd64.deb
  rm -f cri-dockerd_0.3.15.3-0.ubuntu-focal_amd64.deb

  wget https://github.com/kubernetes-sigs/cri-tools/releases/download/$CRICTL_VERSION/crictl-$CRICTL_VERSION-linux-amd64.tar.gz
  sudo tar zxvf crictl-$CRICTL_VERSION-linux-amd64.tar.gz -C /usr/local/bin
  rm -f crictl-$CRICTL_VERSION-linux-amd64.tar.gz
  sudo sysctl fs.protected_regular=0

  CNI_PLUGIN_VERSION="v1.5.1"
  CNI_PLUGIN_TAR="cni-plugins-linux-amd64-$CNI_PLUGIN_VERSION.tgz" # change arch if not on amd64
  CNI_PLUGIN_INSTALL_DIR="/opt/cni/bin"

  curl -LO "https://github.com/containernetworking/plugins/releases/download/$CNI_PLUGIN_VERSION/$CNI_PLUGIN_TAR"
  sudo mkdir -p "$CNI_PLUGIN_INSTALL_DIR"
  sudo tar -xf "$CNI_PLUGIN_TAR" -C "$CNI_PLUGIN_INSTALL_DIR"
  rm "$CNI_PLUGIN_TAR"

  export MINIKUBE_HOME=$HOME CHANGE_MINIKUBE_NONE_USER=true KUBECONFIG=$HOME/.kube/config
  minikube start --kubernetes-version="$1" --driver=none --memory 6g --cpus=2 --addons ingress --cni=calico
}

install_external_snapshotter() {
  EXTERNAL_SNAPSHOTTER_VERSION=8.0.1
  curl -L "https://github.com/kubernetes-csi/external-snapshotter/archive/refs/tags/v${EXTERNAL_SNAPSHOTTER_VERSION}.zip" -o external-snapshotter.zip
  unzip -d /tmp external-snapshotter.zip
  cd "/tmp/external-snapshotter-${EXTERNAL_SNAPSHOTTER_VERSION}"

  kubectl kustomize client/config/crd | kubectl create -f -
  kubectl -n kube-system kustomize deploy/kubernetes/snapshot-controller | kubectl create -f -
}

wait_for_rbd_pvc_clone_to_be_bound() {
  kubectl create -f https://raw.githubusercontent.com/rook/rook/master/deploy/examples/csi/rbd/pvc-clone.yaml

  timeout 100 bash <<-'EOF'
    until [ $(kubectl get pvc rbd-pvc-clone -o jsonpath='{.status.phase}') == "Bound" ]; do
      echo "waiting for the pvc clone to be in bound state"
      sleep 1
    done
EOF
  timeout_command_exit_code
}

########
# MAIN #
########

FUNCTION="$1"
shift # remove function arg now that we've recorded it
# call the function with the remainder of the user-provided args
if ! $FUNCTION "$@"; then
  echo "Call to $FUNCTION was not successful" >&2
  exit 1
fi
