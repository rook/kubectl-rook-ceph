# Flatten RBD PVC

`flatten-rbd-pvc` command is to flatten the RBD image corresponding to the target RBD PVC.
Fore more details about flatten, see [the Ceph official document](https://docs.ceph.com/en/latest/rbd/rbd-snapshot/#flattening-a-cloned-image).

By flattening RBD images, we can bypass the problems specific to non-flattened cloned image like https://github.com/ceph/ceph-csi/discussions/4360.

## Examples.

```bash
kubectl rook-ceph flatten-rbd-pvc rbd-pvc-clone
```
