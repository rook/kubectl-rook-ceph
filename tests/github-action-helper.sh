#!/usr/bin/env bash

set -eEuo pipefail

# source https://github.com/rook/rook
use_local_disk() {
    sudo swapoff --all --verbose
    sudo umount /mnt
    # search for the device since it keeps changing between sda and sdb
    sudo wipefs --all --force /dev/$(lsblk|awk '/14G/ {print $1}'| head -1)1
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
  timeout 20 bash <<-'EOF'
    until [ $(kubectl get pod -l app=rook-ceph-operator -n rook-ceph -o jsonpath='{.items[*].metadata.name}' -o custom-columns=READY:status.containerStatuses[*].ready | grep -c true) -eq 1 ]; do
      echo "waiting for the pods to be in ready state"
      sleep 1
    done
EOF

}
