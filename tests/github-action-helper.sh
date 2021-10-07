#!/usr/bin/env bash

set -eEuo pipefail

: "${FUNCTION:=${1}}"

# source https://krew.sigs.k8s.io/docs/user-guide/setup/install/
install_krew() {
  cd "$(mktemp -d)"
  OS="$(uname | tr '[:upper:]' '[:lower:]')"
  ARCH="$(uname -m | sed -e 's/x86_64/amd64/' -e 's/\(arm\)\(64\)\?.*/\1\2/' -e 's/aarch64$/arm64/')"
  curl -fsSLO "https://github.com/kubernetes-sigs/krew/releases/latest/download/krew.tar.gz"
  tar zxvf krew.tar.gz
  KREW=./krew-"${OS}_${ARCH}"
  "$KREW" install krew
  cp "$HOME"/.krew/bin/kubectl-krew /usr/local/bin/
}

# source https://github.com/rook/rook
use_local_disk() {
  sudo swapoff --all --verbose
  sudo umount /mnt
  # search for the device since it keeps changing between sda and sdb
  sudo wipefs --all --force /dev/"$(lsblk|awk '/14G/ {print $1}'| head -1)"1
  sudo lsblk
}

deploy_rook() {
  kubectl create -f https://raw.githubusercontent.com/rook/rook/master/cluster/examples/kubernetes/ceph/common.yaml
  kubectl create -f https://raw.githubusercontent.com/rook/rook/master/cluster/examples/kubernetes/ceph/crds.yaml
  kubectl create -f https://raw.githubusercontent.com/rook/rook/master/cluster/examples/kubernetes/ceph/operator.yaml
  curl https://raw.githubusercontent.com/rook/rook/master/cluster/examples/kubernetes/ceph/cluster-test.yaml -o cluster-test.yaml
  sed -i "s|#deviceFilter:|deviceFilter: $(lsblk|awk '/14G/ {print $1}'| head -1)|g" cluster-test.yaml
  kubectl create -f cluster-test.yaml
}

# wait_for_pod_to_be_ready_state check for operator pod to in ready state
wait_for_pod_to_be_ready_state() {
  timeout 200 bash <<-'EOF'
    until [ $(kubectl get pod -l app=rook-ceph-osd -n rook-ceph -o jsonpath='{.items[*].metadata.name}' -o custom-columns=READY:status.containerStatuses[*].ready | grep -c true) -eq 1 ]; do
      echo "waiting for the pods to be in ready state"
      sleep 1
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
