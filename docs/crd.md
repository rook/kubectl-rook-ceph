# Restoring Deleted CRs

When a Rook CR is deleted, the Rook operator will respond to the deletion event to attempt to clean up the cluster resources. If any data is still present in the cluster, Rook will refuse to delete the CR to ensure data is not lost. The operator will refuse to remove the finalizer on the CR until the underlying data is deleted.

While the underlying Ceph data and daemons continue to be available, the CRs will be stuck indefinitely in a Deleting state in which the operator will not continue to ensure cluster health. Upgrades will be blocked, further updates to the CRs are prevented, and so on. Since Kubernetes does not allow undeleting resources, the command below will allow repairing the CRs without even necessarily suffering cluster downtime.

> [!NOTE]
> If there are multiple deleted resources in the cluster and no specific resource is mentioned, the first resource will be restored. To restore all deleted resources, re-run the command multiple times.

## restore-deleted Command

The `restore-deleted` command has one required and one optional parameter:

- `<CRD>`: The CRD type that is to be restored, such as CephCluster, CephFilesystem, CephBlockPool and so on.
- `[CRName]`: The name of the specific CR which you want to restore since there can be multiple instances under the same CRD. For example, if there are multiple CephFilesystems stuck in deleting state, a specific filesystem can be restored: `restore-deleted cephfilesystem filesystem-2`.

```bash
kubectl rook-ceph restore-deleted <CRD> [CRName]
```

## CephCluster Restore Example

```bash
kubectl rook-ceph restore-deleted cephcluster

Info: Detecting which resources to restore for crd "cephcluster"
Info: Restoring CR my-cluster
Warning: The resource my-cluster was found deleted. Do you want to restore it? yes | no

Info: skipped prompt since ROOK_PLUGIN_SKIP_PROMPTS=true
Info: Scaling down the operator to 0
Info: Backing up kubernetes and crd resources
Info: Backed up crd cephcluster/my-cluster in file cephcluster-my-cluster.yaml
Info: Deleting validating webhook rook-ceph-webhook if present
Info: Fetching the UID for cephcluster/my-cluster
Info: Successfully fetched uid 8366f79a-ae1f-4679-a62b-8abc6e1528fa from cephcluster/my-cluster
Info: Removing ownerreferences from resources with matching uid 8366f79a-ae1f-4679-a62b-8abc6e1528fa
Info: Removing owner references for secret cluster-peer-token-my-cluster
Info: Removed ownerReference for Secret: cluster-peer-token-my-cluster

Info: Removing owner references for secret rook-ceph-admin-keyring
Info: Removed ownerReference for Secret: rook-ceph-admin-keyring

Info: Removing owner references for secret rook-ceph-config
Info: Removed ownerReference for Secret: rook-ceph-config

Info: Removing owner references for secret rook-ceph-crash-collector-keyring
Info: Removed ownerReference for Secret: rook-ceph-crash-collector-keyring

Info: Removing owner references for secret rook-ceph-mgr-a-keyring
Info: Removed ownerReference for Secret: rook-ceph-mgr-a-keyring

Info: Removing owner references for secret rook-ceph-mons-keyring
Info: Removed ownerReference for Secret: rook-ceph-mons-keyring

Info: Removing owner references for secret rook-csi-cephfs-node
Info: Removed ownerReference for Secret: rook-csi-cephfs-node

Info: Removing owner references for secret rook-csi-cephfs-provisioner
Info: Removed ownerReference for Secret: rook-csi-cephfs-provisioner

Info: Removing owner references for secret rook-csi-rbd-node
Info: Removed ownerReference for Secret: rook-csi-rbd-node

Info: Removing owner references for secret rook-csi-rbd-provisioner
Info: Removed ownerReference for Secret: rook-csi-rbd-provisioner

Info: Removing owner references for configmaps rook-ceph-mon-endpoints
Info: Removed ownerReference for configmap: rook-ceph-mon-endpoints

Info: Removing owner references for service rook-ceph-exporter
Info: Removed ownerReference for service: rook-ceph-exporter

Info: Removing owner references for service rook-ceph-mgr
Info: Removed ownerReference for service: rook-ceph-mgr

Info: Removing owner references for service rook-ceph-mgr-dashboard
Info: Removed ownerReference for service: rook-ceph-mgr-dashboard

Info: Removing owner references for service rook-ceph-mon-a
Info: Removed ownerReference for service: rook-ceph-mon-a

Info: Removing owner references for service rook-ceph-mon-d
Info: Removed ownerReference for service: rook-ceph-mon-d

Info: Removing owner references for service rook-ceph-mon-e
Info: Removed ownerReference for service: rook-ceph-mon-e

Info: Removing owner references for deployemt rook-ceph-mgr-a
Info: Removed ownerReference for deployment: rook-ceph-mgr-a

Info: Removing owner references for deployemt rook-ceph-mon-a
Info: Removed ownerReference for deployment: rook-ceph-mon-a

Info: Removing owner references for deployemt rook-ceph-mon-d
Info: Removed ownerReference for deployment: rook-ceph-mon-d

Info: Removing owner references for deployemt rook-ceph-mon-e
Info: Removed ownerReference for deployment: rook-ceph-mon-e

Info: Removing owner references for deployemt rook-ceph-osd-0
Info: Removed ownerReference for deployment: rook-ceph-osd-0

Info: Removing finalizers from cephcluster/my-cluster
Info: cephcluster.ceph.rook.io/my-cluster patched

Info: Re-creating the CR cephcluster from file cephcluster-my-cluster.yaml created above
Info: cephcluster.ceph.rook.io/my-cluster created

Info: Scaling up the operator to 1
Info: CR is successfully restored. Please watch the operator logs and check the crd
```
