# Destroying a Cluster

When a cluster is no longer needed and needs to be torn down, Rook has a [Cleanup guide](https://rook.io/docs/rook/latest/Getting-Started/ceph-teardown/) with instructions to tear it down cleanly. While following that document is highly preferred, there is a command that will automate the cleanup of Rook after applications are removed.
The `destroy-cluster` command destroys a Rook cluster and deletes CRs (custom resources) by force removing any finalizers that may be preventing the cleanup.

## !!! Warning !!!
**This command is not reversible**, and it will destroy your Rook cluster completely with guaranteed data loss.
Only use this command if you are 100% sure that the cluster and its data should be destroyed.

## `destroy-cluster` command

To destroy a cluster, run the command:
```bash
$ kubectl rook-ceph destroy-cluster
Warning: Are you sure you want to destroy the cluster in namespace "rook-ceph"? If absolutely certain, enter: yes-really-destroy-cluster
```

Any other response will cause the command to abort.

## Example of destroying a cluster
```bash
$ kubectl rook-ceph -n rook-ceph destroy-cluster
Warning: Are you sure you want to destroy the cluster in namespace "rook-ceph"? If absolutely certain, enter: yes-really-destroy-cluster
yes-really-destroy-cluster
Info: proceeding
Info: getting resource kind cephclusters
Info: removing resource cephclusters: my-cluster
Info: resource "my-cluster" is not yet deleted, applying patch to remove finalizer...
Info: resource my-cluster was deleted
Info: getting resource kind cephblockpoolradosnamespaces
Info: resource cephblockpoolradosnamespaces was not found on the cluster
Info: getting resource kind cephblockpools
Info: removing resource cephblockpools: builtin-mgr
Info: resource builtin-mgr was deleted
Info: getting resource kind cephbucketnotifications
Info: resource cephbucketnotifications was not found on the cluster
Info: getting resource kind cephbuckettopics
Info: resource cephbuckettopics was not found on the cluster
Info: getting resource kind cephclients
Info: resource cephclients was not found on the cluster
Info: getting resource kind cephcosidrivers
Info: resource cephcosidrivers was not found on the cluster
Info: getting resource kind cephfilesystemmirrors
Info: resource cephfilesystemmirrors was not found on the cluster
Info: getting resource kind cephfilesystems
Info: removing resource cephfilesystems: myfs
Info: resource myfs was deleted
Info: getting resource kind cephfilesystemsubvolumegroups
Info: removing resource cephfilesystemsubvolumegroups: myfs-csi
Info: resource myfs-csi was deleted
Info: getting resource kind cephnfses
Info: resource cephnfses was not found on the cluster
Info: getting resource kind cephobjectrealms
Info: resource cephobjectrealms was not found on the cluster
Info: getting resource kind cephobjectstores
Info: removing resource cephobjectstores: my-store
Info: resource my-store was deleted
Info: getting resource kind cephobjectstoreusers
Info: resource cephobjectstoreusers was not found on the cluster
Info: getting resource kind cephobjectzonegroups
Info: resource cephobjectzonegroups was not found on the cluster
Info: getting resource kind cephobjectzones
Info: resource cephobjectzones was not found on the cluster
Info: getting resource kind cephrbdmirrors
Info: resource cephrbdmirrors was not found on the cluster
Info: removing deployment rook-ceph-tools
Info: waiting to clean up resources
Info: 1 pods still alive, removing....
Info: pod rook-ceph-mds-myfs-b-58cf7bcbb7-d6rc4 still a live
Info: done
```