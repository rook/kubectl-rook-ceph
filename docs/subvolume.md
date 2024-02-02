# Subvolume cleanup

The subvolume command is used to clean the stale subvolumes
which have no parent-pvc attached to them.
The command would list out all such subvolumes which needs to be removed.
This would consider all the cases where we can have stale subvolume
and delete them without impacting other resources and attached volumes.

The subvolume command will require the following sub commands:
* `ls` : [ls](#ls) lists all the subvolumes
    * `--stale`: lists only stale subvolumes
* `delete <filesystem> <subvolume> [subvolumegroup]`:
    [delete](#delete) a stale subvolume.
    * subvolume: subvolume name.
    * filesystem: filesystem name to which the subvolume belongs.
    * subvolumegroup: subvolumegroup name to which the subvolume belong(default is "csi")
## ls

```bash
kubectl rook-ceph subvolume ls

# Filesystem  Subvolume  SubvolumeGroup  State
# ocs-storagecluster-cephfilesystem csi-vol-427774b4-340b-11ed-8d66-0242ac110004 csi in-use
# ocs-storagecluster-cephfilesystem csi-vol-427774b4-340b-11ed-8d66-0242ac110005 csi in-use
# ocs-storagecluster-cephfilesystem csi-vol-427774b4-340b-11ed-8d66-0242ac110007 csi stale
# ocs-storagecluster-cephfilesystem csi-vol-427774b4-340b-11ed-8d66-0242ac110007 csi stale-with-snapshot

```

```bash
kubectl rook-ceph subvolume ls --stale

# Filesystem  Subvolume  SubvolumeGroup state
# ocs-storagecluster-cephfilesystem csi-vol-427774b4-340b-11ed-8d66-0242ac110004 csi stale
# ocs-storagecluster-cephfilesystem csi-vol-427774b4-340b-11ed-8d66-0242ac110005 csi stale-with-snapshot

```

## delete

```bash
kubectl rook-ceph subvolume delete ocs-storagecluster csi-vol-427774b4-340b-11ed-8d66-0242ac110004

# Info: subvolume "csi-vol-427774b4-340b-11ed-8d66-0242ac110004" deleted

```