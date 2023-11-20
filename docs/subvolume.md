# Subvolume cleanup

The subvolume command is used to clean the stale subvolumes
which have no parent-pvc attached to them.
The command would list out all such subvolumes which needs to be removed.
This would consider all the cases where we can have stale subvolume
and delete them without impacting other resources and attached volumes.

The subvolume command will require the following sub commands:
* `ls` : [ls](#ls) lists all the subvolumes
    * `--stale`: lists only stale subvolumes
* `delete <subvolumes> <filesystem> <subvolumegroup>`:
    [delete](#delete) stale subvolumes as per user's input.
    It will list and delete only the stale subvolumes to prevent any loss of data.
    * subvolumes: comma-separated list of subvolumes of same filesystem and subvolumegroup.
    * filesystem: filesystem name to which the subvolumes belong. 
    * subvolumegroup: subvolumegroup name to which the subvolumes belong.
## ls

```bash
kubectl rook-ceph subvolume ls

# Filesystem  Subvolume  SubvolumeGroup  State
# ocs-storagecluster-cephfilesystem csi-vol-427774b4-340b-11ed-8d66-0242ac110004 csi in-use
# ocs-storagecluster-cephfilesystem csi-vol-427774b4-340b-11ed-8d66-0242ac110005 csi in-use
# ocs-storagecluster-cephfilesystem csi-vol-427774b4-340b-11ed-8d66-0242ac110006 csi in-use
# ocs-storagecluster-cephfilesystem csi-vol-427774b4-340b-11ed-8d66-0242ac110007 csi stale

```

```bash
kubectl rook-ceph subvolume ls --stale

# Filesystem  Subvolume  SubvolumeGroup state
# ocs-storagecluster-cephfilesystem csi-vol-427774b4-340b-11ed-8d66-0242ac110004 csi stale
# ocs-storagecluster-cephfilesystem csi-vol-427774b4-340b-11ed-8d66-0242ac110005 csi stale

```

## delete

```bash
kubectl rook-ceph subvolume delete csi-vol-427774b4-340b-11ed-8d66-0242ac110004 ocs-storagecluster csi

# Info: subvolume csi-vol-427774b4-340b-11ed-8d66-0242ac110004 deleted

```

```bash
kubectl rook-ceph subvolume delete csi-vol-427774b4-340b-11ed-8d66-0242ac110004,csi-vol-427774b4-340b-11ed-8d66-0242ac110005 ocs-storagecluster csi

# Info: subvolume csi-vol-427774b4-340b-11ed-8d66-0242ac110004 deleted
# Info: subvolume csi-vol-427774b4-340b-11ed-8d66-0242ac110004 deleted

```