# Restoring Deleted CRs

When a Rook CR is deleted, the Rook operator will respond to the deletion event to attempt to clean up the cluster resources. If any data is still present in the cluster, Rook will refuse to delete the CR to ensure data is not lost. The operator will refuse to remove the finalizer on the CR until the underlying data is deleted.

While the underlying Ceph data and daemons continue to be available, the CRs will be stuck indefinitely in a Deleting state in which the operator will not continue to ensure cluster health. Upgrades will be blocked, further updates to the CRs are prevented, and so on. Since Kubernetes does not allow undeleting resources, the command below will allow repairing the CRs without even necessarily suffering cluster downtime.

> [!NOTE]
> If there are multiple deleted resources in the cluster and no specific resource is mentioned, the first resource will be restored. To restore all deleted resources, re-run the command multiple times.

## restore-deleted Command

The `restore-deleted` command has one required and one optional parameter:

- `<CRD>`: The CRD type that is to be restored, such as CephClusters, CephFilesystems, CephBlockPools and so on.
- `[CRName]`: The name of the specific CR which you want to restore since there can be multiple instances under the same CRD. For example, if there are multiple CephFilesystems stuck in deleting state, a specific filesystem can be restored: `restore-deleted cephfilesystems filesystem-2`.

```bash
kubectl rook-ceph restore-deleted <CRD> [CRName]
```

## CephClusters Restore Example

```bash
kubectl rook-ceph restore-deleted cephclusters

Info: Detecting which resources to restore for crd "cephclusters"

Info: Restoring CR my-cluster
Warning: The resource my-cluster was found deleted. Do you want to restore it? yes | no

Info: skipped prompt since ROOK_PLUGIN_SKIP_PROMPTS=true
Info: Proceeding with restoring deleting CR
Info: Scaling down the operator
Info: Deleting validating webhook rook-ceph-webhook if present
Info: Removing ownerreferences from resources with matching uid 92c0e549-44fd-43db-80ba-5473db996208
Info: Removing owner references for secret cluster-peer-token-my-cluster
Info: Removed ownerReference for Secret: cluster-peer-token-my-cluster

Info: Removing owner references for secret rook-ceph-admin-keyring
Info: Removed ownerReference for Secret: rook-ceph-admin-keyring

---
---
---

Info: Removing owner references for deployment rook-ceph-osd-0
Info: Removed ownerReference for deployment: rook-ceph-osd-0

Info: Removing finalizers from cephclusters/my-cluster
Info: Re-creating the CR cephclusters from dynamic resource
Info: Scaling up the operator
Info: CR is successfully restored. Please watch the operator logs and check the crd
```
