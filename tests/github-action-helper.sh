#!/usr/bin/env bash

set -eEuo pipefail

: "${FUNCTION:=${1}}"

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
  sudo swapoff --all --verbose
  sudo umount /mnt
  # search for the device since it keeps changing between sda and sdb
  sudo wipefs --all --force /dev/"$(lsblk | awk '/14G/ {print $1}' | head -1)"1
  sudo lsblk
}

deploy_rook() {
  kubectl create -f https://raw.githubusercontent.com/rook/rook/master/deploy/examples/common.yaml
  kubectl create -f https://raw.githubusercontent.com/rook/rook/master/deploy/examples/crds.yaml
  kubectl create -f https://raw.githubusercontent.com/rook/rook/master/deploy/examples/operator.yaml
  curl https://raw.githubusercontent.com/rook/rook/master/deploy/examples/cluster-test.yaml -o cluster-test.yaml
  sed -i "s|#deviceFilter:|deviceFilter: $(lsblk | awk '/14G/ {print $1}' | head -1)|g" cluster-test.yaml
  sed -i '0,/count: 1/ s/count: 1/count: 3/' cluster-test.yaml
  kubectl create -f cluster-test.yaml
  wait_for_pod_to_be_ready_state_default
  kubectl create -f https://raw.githubusercontent.com/rook/rook/master/deploy/examples/csi/rbd/storageclass-test.yaml
  kubectl create -f https://raw.githubusercontent.com/rook/rook/master/deploy/examples/csi/rbd/pvc.yaml
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
  sed -i "s|#deviceFilter:|deviceFilter: $(lsblk | awk '/14G/ {print $1}' | head -1)|g" cluster-test.yaml
  sed -i '0,/count: 1/ s/count: 1/count: 3/' cluster-test.yaml
  deploy_with_custom_ns "$1" "$2" cluster-test.yaml
  wait_for_pod_to_be_ready_state_custom

  curl https://raw.githubusercontent.com/rook/rook/master/deploy/examples/csi/rbd/storageclass-test.yaml -o storageclass-test.yaml
  sed -i "s|provisioner: rook-ceph.rbd.csi.ceph.com |provisioner: test-operator.rbd.csi.ceph.com |g" storageclass-test.yaml
  deploy_with_custom_ns "$1" "$2" storageclass-test.yaml
  kubectl create -f https://raw.githubusercontent.com/rook/rook/master/deploy/examples/csi/rbd/pvc.yaml

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
  timeout 100 bash <<-'EOF'
    until [ $(kubectl -n $namespace get deploy -l app=rook-ceph-mon,mon_canary!=true | grep rook-ceph-mon | wc -l | awk '{print $1}' ) -eq 3 ]; do
      echo "$(date) waiting for three mon deployments to exist"
      sleep 2
    done
EOF
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
