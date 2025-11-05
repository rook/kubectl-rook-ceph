#!/usr/bin/env bash

set -xeEo pipefail

# Source: https://github.com/rook/rook
find_extra_block_dev() {
    # shellcheck disable=SC2005 # redirect doesn't work with sudo, so use echo
    echo "$(sudo lsblk)" >/dev/stderr # print lsblk output to stderr for debugging

    # Find boot device
    local boot_dev
    boot_dev="$(sudo lsblk --noheading --list --output MOUNTPOINT,PKNAME | grep boot | awk '{print $2}')"
    echo "  == find_extra_block_dev(): boot_dev='$boot_dev'" >/dev/stderr

    # Find extra device (--nodeps ignores partitions)
    local extra_dev
    extra_dev="$(sudo lsblk --noheading --list --nodeps --output KNAME | grep -v loop | grep -v "$boot_dev" | head -1)"
    echo "  == find_extra_block_dev(): extra_dev='$extra_dev'" >/dev/stderr

    echo "$extra_dev"
}

#############
# VARIABLES #
#############
: "${FUNCTION:=${1}}"
: "${BLOCK:=$(find_extra_block_dev)}"

# Default namespace values
DEFAULT_OPERATOR_NS="rook-ceph"
DEFAULT_CLUSTER_NS="rook-ceph"

DEFAULT_TIMEOUT=600

# Tool versions
CRICTL_VERSION="v1.31.1"
MINIKUBE_VERSION="v1.35.0"
CNI_PLUGIN_VERSION="v1.6.0"
EXTERNAL_SNAPSHOTTER_VERSION="8.2.0"

# Download YAML files from URLs and modify them for custom namespaces
# Arguments:
#   $1: URL to download from
#   $2: Output file name
#   $3: Operator namespace (optional, defaults to rook-ceph)
#   $4: Cluster namespace (optional, defaults to rook-ceph)
download_and_modify_yaml() {
    local url="$1"
    local output_file="$2"
    local operator_ns="${3:-$DEFAULT_OPERATOR_NS}"
    local cluster_ns="${4:-$DEFAULT_CLUSTER_NS}"

    echo "Downloading $output_file from $url"

    if ! curl -fL "$url" -o "$output_file"; then
        echo "Failed to download $output_file from $url" >&2
        return 1
    fi

    # Apply namespace modifications if not using defaults
    if [[ "$operator_ns" != "$DEFAULT_OPERATOR_NS" || "$cluster_ns" != "$DEFAULT_CLUSTER_NS" ]]; then
        sed -i "s|rook-ceph # namespace:operator|${operator_ns} # namespace:operator|g" "$output_file"
        sed -i "s|rook-ceph # namespace:cluster|${cluster_ns} # namespace:cluster|g" "$output_file"
        # Handle other namespace references that might not have comments
        sed -i "s|namespace: rook-ceph|namespace: ${operator_ns}|g" "$output_file"
    fi
}

# Apply YAML with kubectl
apply_yaml() {
    local file="$1"
    kubectl create -f "$file"
}

# Apply YAML from URL directly
apply_yaml_from_url() {
    local url="$1"
    kubectl create -f "$url"
}

# Source: https://github.com/rook/rook
use_local_disk() {
    local block_data_part="/dev/${BLOCK}1"

    sudo apt purge snapd -y
    sudo dmsetup version || true
    sudo swapoff --all --verbose
    sudo umount /mnt
    sudo wipefs --all --force "$block_data_part"
}

# Deploy Rook-Ceph with support for both default and custom namespaces
# This is the main deployment function that handles the entire process
# Arguments:
#   $1: Operator namespace (optional, defaults to rook-ceph)
#   $2: Cluster namespace (optional, defaults to rook-ceph)
deploy_rook() {
    local operator_ns="${1:-$DEFAULT_OPERATOR_NS}"
    local cluster_ns="${2:-$DEFAULT_CLUSTER_NS}"

    echo "Starting Rook-Ceph deployment"
    echo "Operator namespace: $operator_ns"
    echo "Cluster namespace: $cluster_ns"

    # Create custom namespaces if needed
    if [[ "$operator_ns" != "$DEFAULT_OPERATOR_NS" ]]; then
        echo "Creating operator namespace: $operator_ns"
        kubectl create namespace "$operator_ns" || echo "Namespace $operator_ns already exists"
    fi
    if [[ "$cluster_ns" != "$DEFAULT_CLUSTER_NS" && "$cluster_ns" != "$operator_ns" ]]; then
        echo "Creating cluster namespace: $cluster_ns"
        kubectl create namespace "$cluster_ns" || echo "Namespace $cluster_ns already exists"
    fi

    echo "Deploying Rook common resources..."
    download_and_modify_yaml "https://raw.githubusercontent.com/rook/rook/master/deploy/examples/common.yaml" "common.yaml" "$operator_ns" "$cluster_ns"
    apply_yaml "common.yaml"

    echo "Deploying Custom Resource Definitions (CRDs)..."
    apply_yaml_from_url "https://raw.githubusercontent.com/rook/rook/master/deploy/examples/crds.yaml"

    echo "Deploying Rook operator..."
    download_and_modify_yaml "https://raw.githubusercontent.com/rook/rook/master/deploy/examples/operator.yaml" "operator.yaml" "$operator_ns" "$cluster_ns"
    sed -i "s/ROOK_LOG_LEVEL: \"INFO\"/ROOK_LOG_LEVEL: \"DEBUG\"/g" operator.yaml
    sed -i "s|image: docker.io/rook/ceph:master|image: subham03/rook:rnsn|g" operator.yaml
    apply_yaml "operator.yaml"

    echo "Deploying CSI operator..."
    download_and_modify_yaml "https://raw.githubusercontent.com/rook/rook/master/deploy/examples/csi-operator.yaml" "csi-operator.yaml" "$operator_ns" "$cluster_ns"
    apply_yaml "csi-operator.yaml"

    # Wait for operator to be ready before proceeding
    echo "Waiting for Rook operator to become ready..."
    wait_for_operator_pod_to_be_ready_state "$operator_ns"

    echo "Deploying Ceph cluster with device filter for $BLOCK..."
    download_and_modify_yaml "https://raw.githubusercontent.com/rook/rook/master/deploy/examples/cluster-test.yaml" "cluster-test.yaml" "$operator_ns" "$cluster_ns"
    sed -i "s|#deviceFilter:|deviceFilter: ${BLOCK/\/dev\//}|g" cluster-test.yaml
    sed -i '0,/count: 1/ s/count: 1/count: 3/' cluster-test.yaml
    apply_yaml "cluster-test.yaml"

    echo "Waiting for Ceph cluster to become ready..."
    wait_for_ceph_cluster_to_be_ready_state "$cluster_ns"

    echo "Deploying Ceph toolbox for management..."
    download_and_modify_yaml "https://raw.githubusercontent.com/rook/rook/master/deploy/examples/toolbox.yaml" "toolbox.yaml" "$operator_ns" "$cluster_ns"
    apply_yaml "toolbox.yaml"

    # Phase 3: Deploy storage drivers
    echo "Deploying CSI drivers and storage classes..."
    deploy_csi_drivers "$operator_ns" "$cluster_ns"
}

# Unified CSI driver deployment
deploy_csi_drivers() {
    local operator_ns="${1:-$DEFAULT_OPERATOR_NS}"
    local cluster_ns="${2:-$DEFAULT_CLUSTER_NS}"

    # Deploy filesystem
    download_and_modify_yaml "https://raw.githubusercontent.com/rook/rook/master/deploy/examples/filesystem-test.yaml" "filesystem.yaml" "$operator_ns" "$cluster_ns"
    apply_yaml "filesystem.yaml"

    # Deploy subvolume group
    download_and_modify_yaml "https://raw.githubusercontent.com/rook/rook/master/deploy/examples/subvolumegroup.yaml" "subvolumegroup.yaml" "$operator_ns" "$cluster_ns"
    apply_yaml "subvolumegroup.yaml"

    # Deploy RBD storage class
    download_and_modify_yaml "https://raw.githubusercontent.com/rook/rook/master/deploy/examples/csi/rbd/storageclass-test.yaml" "storageclass-rbd.yaml" "$operator_ns" "$cluster_ns"
    if [[ "$operator_ns" != "$DEFAULT_OPERATOR_NS" ]]; then
        sed -i "s|provisioner: rook-ceph.rbd.csi.ceph.com|provisioner: ${operator_ns}.rbd.csi.ceph.com|g" storageclass-rbd.yaml
    fi
    apply_yaml "storageclass-rbd.yaml"

    # Deploy CephFS storage class
    download_and_modify_yaml "https://raw.githubusercontent.com/rook/rook/master/deploy/examples/csi/cephfs/storageclass.yaml" "storageclass-cephfs.yaml" "$operator_ns" "$cluster_ns"
    if [[ "$operator_ns" != "$DEFAULT_OPERATOR_NS" ]]; then
        sed -i "s|provisioner: rook-ceph.cephfs.csi.ceph.com|provisioner: ${operator_ns}.cephfs.csi.ceph.com|g" storageclass-cephfs.yaml
    fi
    apply_yaml "storageclass-cephfs.yaml"

    # Deploy PVCs
    apply_yaml_from_url "https://raw.githubusercontent.com/rook/rook/master/deploy/examples/csi/rbd/pvc.yaml"
    apply_yaml_from_url "https://raw.githubusercontent.com/rook/rook/master/deploy/examples/csi/cephfs/pvc.yaml"

    # Deploy RADOS namespace components
    deploy_rados_namespace "$operator_ns" "$cluster_ns"
}

# Deploy RADOS namespace components
deploy_rados_namespace() {
    local operator_ns="${1:-$DEFAULT_OPERATOR_NS}"
    local cluster_ns="${2:-$DEFAULT_CLUSTER_NS}"

    # Create block pool for RADOS namespace
    download_and_modify_yaml "https://raw.githubusercontent.com/rook/rook/master/deploy/examples/pool-test.yaml" "pool-rados-ns.yaml" "$operator_ns" "$cluster_ns"
    sed -i "s|name: replicapool|name: blockpool-rados-ns|g" pool-rados-ns.yaml
    apply_yaml "pool-rados-ns.yaml"
    wait_for_cephblockpool_ready_state "$cluster_ns" "blockpool-rados-ns"

    # Create first RADOS namespace (namespace-a)
    download_and_modify_yaml "https://raw.githubusercontent.com/rook/rook/master/deploy/examples/radosnamespace.yaml" "cephblockpoolradosnamespace-namespace-a.yaml" "$operator_ns" "$cluster_ns"
    sed -i "s|blockPoolName: replicapool|blockPoolName: blockpool-rados-ns|g" "cephblockpoolradosnamespace-namespace-a.yaml"
    sed -i "s|name: namespace-a|name: namespace-a|g" "cephblockpoolradosnamespace-namespace-a.yaml"
    apply_yaml "cephblockpoolradosnamespace-namespace-a.yaml"
    # sleep 10 # wait for the rns to be created

    # echo "Deleting rook operator pods to restart for namespace-a..."
    # kubectl delete pods -l app=rook-ceph-operator -n "$operator_ns" --force --grace-period=0 || true

    # echo "Waiting for Rook operator to be ready after restart..."
    # wait_for_operator_pod_to_be_ready_state "$operator_ns"
    wait_for_cephblockpoolradosnamespace_ready_state "$cluster_ns" "namespace-a" "$DEFAULT_TIMEOUT" "$operator_ns"

    # Create second RADOS namespace (namespace-b)
    download_and_modify_yaml "https://raw.githubusercontent.com/rook/rook/master/deploy/examples/radosnamespace.yaml" "cephblockpoolradosnamespace-namespace-b.yaml" "$operator_ns" "$cluster_ns"
    sed -i "s|blockPoolName: replicapool|blockPoolName: blockpool-rados-ns|g" "cephblockpoolradosnamespace-namespace-b.yaml"
    sed -i "s|name: namespace-a|name: namespace-b|g" "cephblockpoolradosnamespace-namespace-b.yaml"
    apply_yaml "cephblockpoolradosnamespace-namespace-b.yaml"
    # sleep 10 # wait for the rns to be created
    # echo "Deleting rook operator pods to restart for namespace-b..."
    # kubectl delete pods -l app=rook-ceph-operator -n "$operator_ns" --force --grace-period=0 || true

    # echo "Waiting for Rook operator to be ready after restart..."
    # wait_for_operator_pod_to_be_ready_state "$operator_ns"
    wait_for_cephblockpoolradosnamespace_ready_state "$cluster_ns" "namespace-b" "$DEFAULT_TIMEOUT" "$operator_ns"


    # Get cluster ID and create storage class
    local cluster_id
    cluster_id=$(kubectl -n "$cluster_ns" get cephblockpoolradosnamespace/namespace-a -o jsonpath='{.status.info.clusterID}')
    echo "cluster_id=${cluster_id}"

    download_and_modify_yaml "https://raw.githubusercontent.com/rook/rook/master/deploy/examples/csi/rbd/storageclass-test.yaml" "storageclass-rados-namespace.yaml" "$operator_ns" "$cluster_ns"
    sed -i "s|clusterID: rook-ceph # namespace:cluster|clusterID: ${cluster_id}|g" storageclass-rados-namespace.yaml
    sed -i "s|name: rook-ceph-block|name: rook-ceph-block-rados-namespace|g" storageclass-rados-namespace.yaml
    sed -i "s|pool: replicapool|pool: blockpool-rados-ns|g" storageclass-rados-namespace.yaml
    kubectl apply -f storageclass-rados-namespace.yaml

    # Create PVC for RADOS namespace
    if ! curl -fL "https://raw.githubusercontent.com/rook/rook/master/deploy/examples/csi/rbd/pvc.yaml" -o pvc-rados-namespace.yaml; then
        echo "Failed to download PVC template for RADOS namespace" >&2
        return 1
    fi
    sed -i "s|name: rbd-pvc|name: rbd-pvc-rados-namespace|g" pvc-rados-namespace.yaml
    sed -i "s|storageClassName: rook-ceph-block|storageClassName: rook-ceph-block-rados-namespace|g" pvc-rados-namespace.yaml
    apply_yaml "pvc-rados-namespace.yaml"
}

# Create storage class with retain policy
create_sc_with_retain_policy() {
    local operator_ns="${1:-$DEFAULT_OPERATOR_NS}"
    local cluster_ns="${2:-$DEFAULT_CLUSTER_NS}"

    download_and_modify_yaml "https://raw.githubusercontent.com/rook/rook/master/deploy/examples/csi/cephfs/storageclass.yaml" "storageclass-retain.yaml" "$operator_ns" "$cluster_ns"
    sed -i "s|name: rook-cephfs|name: rook-cephfs-retain|g" storageclass-retain.yaml
    sed -i "s|reclaimPolicy: Delete|reclaimPolicy: Retain|g" storageclass-retain.yaml
    if [[ "$operator_ns" != "$DEFAULT_OPERATOR_NS" ]]; then
        sed -i "s|provisioner: rook-ceph.cephfs.csi.ceph.com|provisioner: ${operator_ns}.cephfs.csi.ceph.com|g" storageclass-retain.yaml
    fi
    apply_yaml "storageclass-retain.yaml"
}

# Create stale subvolume for testing
create_stale_subvolume() {
    if ! curl -fL "https://raw.githubusercontent.com/rook/rook/master/deploy/examples/csi/cephfs/pvc.yaml" -o pvc-retain.yaml; then
        echo "Failed to download PVC retain template" >&2
        return 1
    fi
    sed -i "s|name: cephfs-pvc|name: cephfs-pvc-retain|g" pvc-retain.yaml
    sed -i "s|storageClassName: rook-cephfs|storageClassName: rook-cephfs-retain|g" pvc-retain.yaml
    apply_yaml "pvc-retain.yaml"

    kubectl get pvc cephfs-pvc-retain
    wait_for_pvc_to_be_bound_state "cephfs-pvc-retain" "default"

    local pv_name
    pv_name="$(kubectl get pvc cephfs-pvc-retain -o=jsonpath='{.spec.volumeName}')"
    kubectl delete pvc cephfs-pvc-retain
    kubectl delete pv "$pv_name"
}


#################
# WAIT FUNCTIONS
#################

wait_for_cephblockpoolradosnamespace_ready_state() {
    local cluster_ns="$1"
    local namespace_name="$2"
    local timeout_duration="${3:-$DEFAULT_TIMEOUT}"

    echo "Waiting for CephBlockPoolRadosNamespace '$namespace_name' to be ready..."
    if ! kubectl wait --for=jsonpath='{.status.phase}'=Ready CephBlockPoolRadosNamespace "$namespace_name" -n "$cluster_ns" --timeout="${timeout_duration}s"; then
        echo "Timeout waiting for CephBlockPoolRadosNamespace $namespace_name" >&2
        echo "Current RADOS namespaces:"
        kubectl get CephBlockPoolRadosNamespace -A
        exit 1
    fi
    echo "CephBlockPoolRadosNamespace $namespace_name is ready!"
}

# Wait for CephBlockPool to be ready
wait_for_cephblockpool_ready_state() {
    local cluster_ns="$1"
    local blockpool_name="$2"
    local timeout_duration="${3:-$DEFAULT_TIMEOUT}"

    echo "Waiting for CephBlockPool $blockpool_name to be in Ready phase"
    if ! kubectl wait --for=jsonpath='{.status.phase}'=Ready CephBlockPool "$blockpool_name" -n "$cluster_ns" --timeout="${timeout_duration}s"; then
        echo "Timeout waiting for CephBlockPool $blockpool_name" >&2
        kubectl get CephBlockPool -A
        kubectl describe CephBlockPool "$blockpool_name" -n "$cluster_ns"
        exit 1
    fi
    echo "CephBlockPool $blockpool_name is in Ready phase!"
}

# Wait for PVC to be bound
wait_for_pvc_to_be_bound_state() {
    local pvc_name="${1:-cephfs-pvc-retain}"
    local namespace="${2:-default}"

    echo "Waiting for PVC $pvc_name to be in bound state"
    kubectl wait --for=jsonpath='{.status.phase}'=Bound pvc "$pvc_name" -n "$namespace" --timeout=${DEFAULT_TIMEOUT}s
}

# Wait for ceph cluster to be ready
wait_for_ceph_cluster_to_be_ready_state() {
    local cluster_ns="$1"

    echo "Waiting for CephCluster to be ready in namespace $cluster_ns"
    if ! kubectl wait --for=condition=Ready cephcluster my-cluster -n "$cluster_ns" --timeout=${DEFAULT_TIMEOUT}s; then
        echo "CephCluster failed to become ready, current status:"
        kubectl get cephcluster -A
        exit 1
    fi
}

# Wait for operator pod to be ready
wait_for_operator_pod_to_be_ready_state() {
    local operator_ns="$1"

    echo "Waiting for operator pod to be ready in namespace $operator_ns"
    kubectl wait --for=condition=Ready pod -l app=rook-ceph-operator -n "$operator_ns" --timeout=${DEFAULT_TIMEOUT}s
}

wait_for_three_mons() {
  local namespace=$1
  timeout 600 bash <<EOF
    set -x
    until [ \$(kubectl -n $namespace get deploy -l app=rook-ceph-mon,mon_canary!=true | grep rook-ceph-mon | wc -l | awk '{print \$1}' ) -eq 3 ]; do
      echo "\$(date) waiting for three mon deployments to exist"
      sleep 2
    done
EOF
}

wait_for_deployment_to_be_running() {
    local deployment="$1"
    local namespace="$2"

    if [[ -z "$deployment" || -z "$namespace" ]]; then
        echo "Error: Both deployment and namespace parameters are required" >&2
        return 1
    fi

    echo "Waiting for deployment '$deployment' to be available in namespace '$namespace'..."
    kubectl -n "$namespace" wait deployment "$deployment" --for condition=Available=True --timeout=${DEFAULT_TIMEOUT}s
}

install_minikube_with_none_driver() {
    local kubernetes_version="$1"

    if [[ -z "$kubernetes_version" ]]; then
        echo "Error: Kubernetes version must be specified" >&2
        echo "Usage: install_minikube_with_none_driver <k8s-version>" >&2
        return 1
    fi

    echo "Installing minikube with Kubernetes $kubernetes_version"

    # Install system dependencies
    echo "Installing system dependencies..."
    sudo apt update
    sudo apt install -y conntrack socat

    # Install minikube
    echo "Installing minikube $MINIKUBE_VERSION..."
    if ! curl -LO "https://storage.googleapis.com/minikube/releases/$MINIKUBE_VERSION/minikube_latest_amd64.deb"; then
        echo "Failed to download minikube package" >&2
        return 1
    fi
    sudo dpkg -i minikube_latest_amd64.deb
    rm -f minikube_latest_amd64.deb

    # Install container runtime interface
    echo "Installing cri-dockerd..."
    if ! curl -LO "https://github.com/Mirantis/cri-dockerd/releases/download/v0.3.15/cri-dockerd_0.3.15.3-0.ubuntu-focal_amd64.deb"; then
        echo "Failed to download cri-dockerd" >&2
        return 1
    fi
    sudo dpkg -i "cri-dockerd_0.3.15.3-0.ubuntu-focal_amd64.deb"
    rm -f "cri-dockerd_0.3.15.3-0.ubuntu-focal_amd64.deb"


    # Install crictl (container runtime CLI)
    echo "Installing crictl $CRICTL_VERSION..."
    if ! curl -LO "https://github.com/kubernetes-sigs/cri-tools/releases/download/$CRICTL_VERSION/crictl-$CRICTL_VERSION-linux-amd64.tar.gz"; then
        echo "Failed to download crictl" >&2
        return 1
    fi
    sudo tar zxf "crictl-$CRICTL_VERSION-linux-amd64.tar.gz" -C /usr/local/bin
    rm -f "crictl-$CRICTL_VERSION-linux-amd64.tar.gz"

    # Configure system settings
    echo "Configuring system settings..."
    sudo sysctl fs.protected_regular=0

    # Install CNI plugins
    echo "Installing CNI plugins $CNI_PLUGIN_VERSION..."
    local cni_plugin_tar="cni-plugins-linux-amd64-$CNI_PLUGIN_VERSION.tgz"
    local cni_plugin_install_dir="/opt/cni/bin"

    if ! curl -LO "https://github.com/containernetworking/plugins/releases/download/$CNI_PLUGIN_VERSION/$cni_plugin_tar"; then
        echo "Failed to download CNI plugins" >&2
        return 1
    fi
    sudo mkdir -p "$cni_plugin_install_dir"
        sudo tar -xf "$cni_plugin_tar" -C "$cni_plugin_install_dir"
        rm "$cni_plugin_tar"

    # Start minikube cluster
    echo "Starting minikube cluster..."
    export MINIKUBE_HOME=$HOME CHANGE_MINIKUBE_NONE_USER=true KUBECONFIG=$HOME/.kube/config
    minikube start \
        --kubernetes-version="$kubernetes_version" \
        --driver=none \
        --memory 6g \
        --cpus=2 \
        --addons ingress \
        --cni=calico

    echo "Minikube installation completed successfully!"
}

# Install external snapshotter
install_external_snapshotter() {
    echo "Installing external snapshotter version $EXTERNAL_SNAPSHOTTER_VERSION..."

    if ! curl -L "https://github.com/kubernetes-csi/external-snapshotter/archive/refs/tags/v${EXTERNAL_SNAPSHOTTER_VERSION}.zip" -o external-snapshotter.zip; then
        echo "Failed to download external snapshotter" >&2
        return 1
    fi

    unzip -d /tmp external-snapshotter.zip
    cd "/tmp/external-snapshotter-${EXTERNAL_SNAPSHOTTER_VERSION}"

    kubectl kustomize client/config/crd | kubectl create -f -
    kubectl -n kube-system kustomize deploy/kubernetes/snapshot-controller | kubectl create -f -
}

# Wait for RBD PVC clone to be bound
wait_for_rbd_pvc_clone_to_be_bound() {
    echo "Creating RBD PVC clone..."
    apply_yaml_from_url "https://raw.githubusercontent.com/rook/rook/master/deploy/examples/csi/rbd/pvc-clone.yaml"

    echo "Waiting for PVC clone to be bound..."
    kubectl wait --for=jsonpath='{.status.phase}'=Bound pvc rbd-pvc-clone --timeout=600s
}

########
# MAIN #
########

FUNCTION="$1"
shift # remove function arg now that we've recorded it

# Call the function with the remainder of the user-provided args
if ! "$FUNCTION" "$@"; then
    echo "Function '$FUNCTION' failed" >&2
    exit 1
fi

echo "Function '$FUNCTION' completed successfully!"
