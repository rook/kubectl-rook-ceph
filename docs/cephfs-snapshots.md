# Snapshot cleanup

The snapshot command is used to manage and clean up orphaned CephFS snapshots
which have no corresponding Kubernetes VolumeSnapshotContent resource.
The command lists all snapshots in a subvolumegroup and identifies their status as bound or orphaned.
Orphaned snapshots can then be safely deleted without impacting active resources.

The snapshot command will require the following sub commands:

* `ls` : [ls](#ls) lists all the snapshots
  * `--orphaned`: lists only orphaned snapshots
  * `--svg <subvolumegroupname>`: lists snapshots in a particular subvolume group (default is "csi")
* `delete <filesystem> <subvolume> <snapshot>`:
    [delete](#delete) an orphaned snapshot.
  * filesystem: filesystem name to which the snapshot belongs.
  * subvolume: subvolume name to which the snapshot belongs.
  * snapshot: snapshot name.
  * `--svg <subvolumegroupname>`: subvolume group name (default is "csi")

## ls

```bash
$ kubectl rook-ceph snapshot ls

Filesystem                         Subvolume                                     SubvolumeGroup  Snapshot                                       State
ocs-storagecluster-cephfilesystem  csi-vol-aa0099b5-f7a0-49c2-bc97-a810005a9654  csi             csi-snap-3936435c-a14a-4a76-9d0f-71321ac084a9  bound
ocs-storagecluster-cephfilesystem  csi-vol-aa0099b5-f7a0-49c2-bc97-a810005a9654  csi             csi-snap-3936435c-a14a-4a76-9d0f-71321ac084a9  orphaned
```

```bash
$ kubectl rook-ceph snapshot ls --orphaned

Filesystem                         Subvolume                                     SubvolumeGroup  Snapshot                                       State
ocs-storagecluster-cephfilesystem  csi-vol-aa0099b5-f7a0-49c2-bc97-a810005a9654  csi             csi-snap-3936435c-a14a-4a76-9d0f-71321ac084a9  orphaned
```

## delete

```bash
$ kubectl rook-ceph snapshot delete ocs-storagecluster-cephfilesystem csi-vol-427774b4-340b-11ed-8d66-0242ac110005 csi-snap-b2c3d4e5-450e-11ed-8d66-0242ac110005

Info: Deleting the omap object and key for snapshot "csi-snap-b2c3d4e5-450e-11ed-8d66-0242ac110005"
Info: omap object:"csi.snap.b2c3d4e5-450e-11ed-8d66-0242ac110005" deleted
Info: omap key:"csi.snap.snapshot-a1b2c3d4-5678-9012-abcd-ef0123456789" deleted
snapshot csi-snap-b2c3d4e5-450e-11ed-8d66-0242ac110005 deleted successfully
```

```bash
$ kubectl rook-ceph snapshot delete myfs csi-vol-427774b4-340b-11ed-8d66-0242ac110005 csi-snap-a1b2c3d4-450e-11ed-8d66-0242ac110004

Error: snapshot "csi-snap-a1b2c3d4-450e-11ed-8d66-0242ac110004" is bound and cannot be deleted
```
