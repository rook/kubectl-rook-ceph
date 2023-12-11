#!/usr/bin/env bash

set -xeEo pipefail

#############
# VARIABLES #
#############
: "${FUNCTION:=${1}}"
: "${BLOCK:=$(sudo lsblk --paths | awk '/14G/ || /64G/ {print $1}' | head -1)}"

# source https://krew.sigs.k8s.io/docs/user-guide/setup/install/
install_krew() {
  cd "$(mktemp -d)"
  OS="$(uname | tr '[:upper:]' '[:lower:]')"
  ARCH="$(uname -m | sed -e 's/x86_64/amd64/' -e 's/\(arm\)\(64\)\?.*/\1\2/' -e 's/aarch64$/arm64/')"
  KREW="krew-${OS}_${ARCH}"
  curl -fsSLO "https://github.com/kubernetes-sigs/krew/releases/latest/download/${KREW}.tar.gz"
  tar zxvf "${KREW}.tar.gz"
  ./"${KREW}" install krew
  cp "$HOME"/.krew/bin/kubectl-krew /usr/local/bin/
  kubectl krew install rook-ceph
}

# source https://github.com/rook/rook
use_local_disk() {
  BLOCK_DATA_PART=${BLOCK}1
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
  curl https://raw.githubusercontent.com/rook/rook/master/deploy/examples/cluster-test.yaml -o cluster-test.yaml
  sed -i "s|#deviceFilter:|deviceFilter: ${BLOCK/\/dev\//}|g" cluster-test.yaml
  sed -i '0,/count: 1/ s/count: 1/count: 3/' cluster-test.yaml
  kubectl create -f cluster-test.yaml
  wait_for_pod_to_be_ready_state_default
  kubectl create -f https://raw.githubusercontent.com/rook/rook/master/deploy/examples/filesystem-test.yaml
  kubectl create -f https://raw.githubusercontent.com/rook/rook/master/deploy/examples/subvolumegroup.yaml
  kubectl create -f https://raw.githubusercontent.com/rook/rook/master/deploy/examples/toolbox.yaml
  kubectl create -f https://raw.githubusercontent.com/rook/rook/master/deploy/examples/csi/rbd/storageclass-test.yaml
  kubectl create -f https://raw.githubusercontent.com/rook/rook/master/deploy/examples/csi/rbd/pvc.yaml
  kubectl create -f https://raw.githubusercontent.com/rook/rook/master/deploy/examples/csi/cephfs/pvc.yaml
}

deploy_rook_in_custom_namespace() {
  OPERATOR_NS=$1
  CLUSTER_NS=$2
  : "${OPERATOR_NS:=test-operator}"
  : "${CLUSTER_NS:=test-cluster}"

  kubectl create namespace test-operator # creating namespace manually because rook common.yaml create one namespace and here we need 2
  curl https://raw.githubusercontent.com/rook/rook/master/deploy/examples/common.yaml -o common.yaml
  deploy_with_custom_ns "$1" "$2" common.yaml
  kubectl create -f https://raw.githubusercontent.com/rook/rook/master/deploy/examples/crds.yaml
  curl -f https://raw.githubusercontent.com/rook/rook/master/deploy/examples/operator.yaml -o operator.yaml
  deploy_with_custom_ns "$1" "$2" operator.yaml
  curl https://raw.githubusercontent.com/rook/rook/master/deploy/examples/cluster-test.yaml -o cluster-test.yaml
  sed -i "s|#deviceFilter:|deviceFilter: ${BLOCK/\/dev\//}|g" cluster-test.yaml
  sed -i '0,/count: 1/ s/count: 1/count: 3/' cluster-test.yaml
  deploy_with_custom_ns "$1" "$2" cluster-test.yaml
  wait_for_pod_to_be_ready_state_custom

  curl https://raw.githubusercontent.com/rook/rook/master/deploy/examples/csi/rbd/storageclass-test.yaml -o storageclass-test.yaml
  sed -i "s|provisioner: rook-ceph.rbd.csi.ceph.com |provisioner: test-operator.rbd.csi.ceph.com |g" storageclass-test.yaml
  deploy_with_custom_ns "$1" "$2" storageclass-test.yaml
  kubectl create -f https://raw.githubusercontent.com/rook/rook/master/deploy/examples/csi/rbd/pvc.yaml
  curl -f https://raw.githubusercontent.com/rook/rook/master/deploy/examples/filesystem-test.yaml -o filesystem.yaml
  deploy_with_custom_ns "$1" "$2" filesystem.yaml
  curl -f https://raw.githubusercontent.com/rook/rook/master/deploy/examples/subvolumegroup.yaml -o subvolumegroup.yaml
  deploy_with_custom_ns "$1" "$2" subvolumegroup.yaml
  kubectl create -f https://raw.githubusercontent.com/rook/rook/master/deploy/examples/csi/cephfs/pvc.yaml

  curl -f https://raw.githubusercontent.com/rook/rook/master/deploy/examples/toolbox.yaml -o toolbox.yaml
  deploy_with_custom_ns "$1" "$2" toolbox.yaml
}

deploy_with_custom_ns() {
  sed -i "s|rook-ceph # namespace:operator|$1 # namespace:operator|g" "$3"
  sed -i "s|rook-ceph # namespace:cluster|$2 # namespace:cluster|g" "$3"
  kubectl create -f "$3"
}

wait_for_pod_to_be_ready_state_default() {
  timeout 200 bash <<-'EOF'
    until [ $(kubectl get pod -l app=rook-ceph-osd -n rook-ceph -o jsonpath='{.items[*].metadata.name}' -o custom-columns=READY:status.containerStatuses[*].ready | grep -c true) -eq 1 ]; do
      echo "waiting for the pods to be in ready state"
      sleep 1
    done
EOF
}

wait_for_pod_to_be_ready_state_custom() {
  timeout 200 bash <<-'EOF'
    until [ $(kubectl get pod -l app=rook-ceph-osd -n test-cluster -o jsonpath='{.items[*].metadata.name}' -o custom-columns=READY:status.containerStatuses[*].ready | grep -c true) -eq 1 ]; do
      echo "waiting for the pods to be in ready state"
      sleep 1
    done
EOF
}

wait_for_operator_pod_to_be_ready_state_default() {
  timeout 100 bash <<-'EOF'
    until [ $(kubectl get pod -l app=rook-ceph-operator -n rook-ceph -o jsonpath='{.items[*].metadata.name}' -o custom-columns=READY:status.containerStatuses[*].ready | grep -c true) -eq 1 ]; do
      echo "waiting for the operator to be in ready state"
      sleep 1
    done
EOF
}

wait_for_operator_pod_to_be_ready_state_custom() {
  timeout 100 bash <<-'EOF'
    until [ $(kubectl get pod -l app=rook-ceph-operator -n test-operator -o jsonpath='{.items[*].metadata.name}' -o custom-columns=READY:status.containerStatuses[*].ready | grep -c true) -eq 1 ]; do
      echo "waiting for the operator to be in ready state"
      sleep 1
    done
EOF
}

wait_for_three_mons() {
  export namespace=$1
  timeout 150 bash <<-'EOF'
    until [ $(kubectl -n $namespace get deploy -l app=rook-ceph-mon,mon_canary!=true | grep rook-ceph-mon | wc -l | awk '{print $1}' ) -eq 3 ]; do
      echo "$(date) waiting for three mon deployments to exist"
      sleep 2
    done
EOF
}

wait_for_deployment_to_be_running() {
  deployment=$1
  namespace=$2
  echo "Waiting for the pod from deployment \"$deployment\" to be running"
  kubectl -n "$namespace" wait deployment "$deployment" --for condition=Available=True --timeout=90s
}

wait_for_crd_to_be_ready_default() {
  timeout 150 bash <<-'EOF'
    until [ $(kubectl -n rook-ceph get cephcluster my-cluster -o=jsonpath='{.status.phase}') == "Ready" ]; do
      echo "Waiting for the CephCluster my-cluster to be in the Ready state..."
      sleep 2
    done
EOF
}

wait_for_crd_to_be_ready_custom() {
  timeout 150 bash <<-'EOF'
    until [ $(kubectl -n test-cluster get cephcluster my-cluster -o=jsonpath='{.status.phase}') == "Ready" ]; do
      echo "Waiting for the CephCluster my-cluster to be in the Ready state..."
      sleep 2
    done
EOF
}

install_minikube_with_none_driver() {
  CRICTL_VERSION="v1.28.0"
  MINIKUBE_VERSION="v1.31.2"

  sudo apt update
  sudo apt install -y conntrack socat
  curl -LO https://storage.googleapis.com/minikube/releases/$MINIKUBE_VERSION/minikube_latest_amd64.deb
  sudo dpkg -i minikube_latest_amd64.deb
  rm -f minikube_latest_amd64.deb

  curl -LO https://github.com/Mirantis/cri-dockerd/releases/download/v0.3.4/cri-dockerd_0.3.4.3-0.ubuntu-focal_amd64.deb
  sudo dpkg -i cri-dockerd_0.3.4.3-0.ubuntu-focal_amd64.deb
  rm -f cri-dockerd_0.3.4.3-0.ubuntu-focal_amd64.deb

  wget https://github.com/kubernetes-sigs/cri-tools/releases/download/$CRICTL_VERSION/crictl-$CRICTL_VERSION-linux-amd64.tar.gz
  sudo tar zxvf crictl-$CRICTL_VERSION-linux-amd64.tar.gz -C /usr/local/bin
  rm -f crictl-$CRICTL_VERSION-linux-amd64.tar.gz
  sudo sysctl fs.protected_regular=0

  CNI_PLUGIN_VERSION="v1.3.0"
  CNI_PLUGIN_TAR="cni-plugins-linux-amd64-$CNI_PLUGIN_VERSION.tgz" # change arch if not on amd64
  CNI_PLUGIN_INSTALL_DIR="/opt/cni/bin"

  curl -LO "https://github.com/containernetworking/plugins/releases/download/$CNI_PLUGIN_VERSION/$CNI_PLUGIN_TAR"
  sudo mkdir -p "$CNI_PLUGIN_INSTALL_DIR"
  sudo tar -xf "$CNI_PLUGIN_TAR" -C "$CNI_PLUGIN_INSTALL_DIR"
  rm "$CNI_PLUGIN_TAR"

  export MINIKUBE_HOME=$HOME CHANGE_MINIKUBE_NONE_USER=true KUBECONFIG=$HOME/.kube/config
  sudo -E minikube start --kubernetes-version="$1" --driver=none --memory 6g --cpus=2 --addons ingress --cni=calico
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
