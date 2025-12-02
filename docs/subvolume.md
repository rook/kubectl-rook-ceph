# Subvolume cleanup

The subvolume command is used to clean the stale subvolumes
which have no parent-pvc attached to them.
The command would list out all such subvolumes which needs to be removed.
This would consider all the cases where we can have stale subvolume
and delete them without impacting other resources and attached volumes.

The subvolume command will require the following sub commands:

* `ls` : [ls](#ls) lists all the subvolumes
  * `--stale`: lists only stale subvolumes
  * `--svg <subvolumegroupname>`: lists subvolumes in a particular subvolume(default is "csi")
* `delete <filesystem> <subvolume> [subvolumegroup]`:
    [delete](#delete) a stale subvolume.
  * subvolume: subvolume name.
  * filesystem: filesystem name to which the subvolume belongs.
  * subvolumegroup: subvolumegroup name to which the subvolume belong(default is "csi")

## ls

```bash
$ kubectl rook-ceph subvolume ls

Filesystem  Subvolume  SubvolumeGroup  State
myfs csi-vol-427774b4-340b-11ed-8d66-0242ac110004 csi in-use
myfs csi-vol-427774b4-340b-11ed-8d66-0242ac110005 csi in-use
myfs csi-vol-427774b4-340b-11ed-8d66-0242ac110007 csi stale
myfs csi-vol-427774b4-340b-11ed-8d66-0242ac110007 csi stale-with-snapshot
```

```bash
$ kubectl rook-ceph subvolume ls --stale

Filesystem  Subvolume  SubvolumeGroup state
myfs csi-vol-427774b4-340b-11ed-8d66-0242ac110004 csi stale
myfs csi-vol-427774b4-340b-11ed-8d66-0242ac110005 csi stale-with-snapshot
```

```bash
$ kubectl rook-ceph subvolume ls --svg svg01
myfs csi-vol-427774b4-340b-11ed-8d66-0242ac110005 svg01 in-use
myfs csi-vol-427774b4-340b-11ed-8d66-0242ac110007 svg01 stale
```

## delete

```bash
$ kubectl rook-ceph subvolume delete myfs csi-vol-427774b4-340b-11ed-8d66-0242ac110005

Info: Deleting the omap object and key for subvolume "csi-vol-0c91ba82-5a63-4117-88a4-690acd86cbbd"
Info: omap object:"csi.volume.0c91ba82-5a63-4117-88a4-690acd86cbbd" deleted
Info: omap key:"csi.volume.pvc-78abf81c-5381-42ee-8d75-dc17cd0cf5de" deleted
Info: subvolume "csi-vol-0c91ba82-5a63-4117-88a4-690acd86cbbd" deleted
```

```bash
$ kubectl rook-ceph subvolume delete myfs csi-vol-427774b4-340b-11ed-8d66-0242ac110005

Info: No omapvals found for subvolume csi-vol-427774b4-340b-11ed-8d66-0242ac110005
Info: subvolume "csi-vol-427774b4-340b-11ed-8d66-0242ac110005" deleted
```
