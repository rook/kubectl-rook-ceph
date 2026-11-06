# Preflight

The `preflight` command checks whether a Kubernetes cluster's nodes can run a Rook Ceph cluster,
before the cluster is created. It needs no manifests: it compares the nodes with Rook's defaults.

Unlike other commands, `preflight` does not need Rook to be installed.

It validates the following:

1. **Kubernetes Version**: the server version is at least the minimum that the latest version of
   Rook supports. The minimum is read from the
   [Rook prerequisites](https://rook.io/docs/rook/latest/Getting-Started/Prerequisites/prerequisites/).
2. **Node Architecture**: every node is `amd64` or `arm64`.
3. **Mon Placement**: there are at least three schedulable nodes, so Rook's default of three mons
   can run on different nodes. Nodes that are not ready, are cordoned, or have a
   `NoSchedule` or `NoExecute` taint are not counted.
4. **Kernel**: the kernel is 5.11 or newer, which the kernel RBD and CephFS clients need to connect
   with msgr2, the default since [rook/rook#18090](https://github.com/rook/rook/issues/18090), and
   the `rbd` module is loaded, built in, or installed.
5. **Disks for OSDs**: the disks on each node that Rook can use for OSDs, with the reason each other
   disk is skipped. Nodes without usable disks are listed for information; the check is critical
   only if no node has a usable disk, and a warning if fewer than three nodes have one, since Ceph
   keeps three replicas on different hosts by default. Rook creates OSDs only on the disks selected in the
   CephCluster storage settings, so not every usable disk has to become an OSD.
6. **Node Memory**: each node's allocatable memory would cover a mon and an OSD on every usable disk
   (4GiB each, the default `osd_memory_target`). This is an upper estimate.

Checks 4 to 6 inspect the nodes with a short-lived privileged pod on each schedulable node. The pod
uses the Ceph image and runs `ceph-volume inventory`, the same inventory that the OSD prepare job
uses to decide which disks it can use. If the pods cannot be created or do not finish, the checks
that need them report the error, for example when RBAC does not allow privileged pods. Pass
`--skip-node-inspection` to run checks 1 to 4 without any pods (the `rbd` module is then not
checked). The pods are deleted when the command finishes.

For clusters whose OSDs run on PVCs, such as in the cloud, pass `--pvc-cluster` to skip checks 5
and 6. The kernel and `rbd` module are still checked. Combine it with `--skip-node-inspection` to
create no pods at all.

The command exits with status 1 if any check is critical or fails, so it can be used in scripts.

## Usage

```bash
kubectl rook-ceph preflight
kubectl rook-ceph preflight -o json
kubectl rook-ceph preflight --skip-node-inspection
kubectl rook-ceph preflight --pvc-cluster
```

Flags:

- `--image`: Ceph image for the node inspection pods. Defaults to `quay.io/ceph/ceph:v20.2.4`, the
    default Ceph image of Rook v1.21.
- `--probe-namespace`: namespace for the node inspection pods. Defaults to the cluster namespace
    (`-n`, `rook-ceph` by default) if it exists, otherwise `default`. The namespace must allow
    privileged pods.
- `--node-selector`: label selector for the nodes to check.
- `--skip-node-inspection`: do not create pods, and skip the disk, `rbd` and memory checks.
- `--pvc-cluster`: the OSDs will run on PVCs; skip the disk and memory checks.
- `-o`, `--output`: `text` (default), `json` or `yaml`.
- `--verbose`: show the details of each check, such as why a disk is not usable and how to set up a
    test cluster, and list every node and disk.

## Example Output

On a single node with two empty disks:

```console
$ kubectl rook-ceph preflight
Checking Kubernetes Version...
Checking Node Architecture...
Checking Mon Placement...
Inspecting 1 node(s) with image quay.io/ceph/ceph:v20.2.4 in namespace rook-ceph...
Checking Kernel...
Checking Disks for OSDs...
Checking Node Memory...

========================================================================
PREFLIGHT REPORT
========================================================================
Generated: 2026-09-27 17:33:28 UTC
Namespace: rook-ceph

========================================================================
Storage
========================================================================

[!!] Disks for OSDs [WARNING]
	Status: 2 usable disk(s) on 1 node(s)

[OK] Kernel [OK]
	Status: Kernel versions support msgr2 and the rbd module is available

========================================================================
K8s Resources
========================================================================

[!!] Mon Placement [WARNING]
	Status: Only 1 node(s) are schedulable; Rook's default of 3 mons needs 3 nodes

[!!] Node Memory [WARNING]
	Status: 1 node(s) may not have enough memory for a mon and an OSD on every usable disk

[OK] Kubernetes Version [OK]
	Status: Kubernetes v1.37.0

[OK] Node Architecture [OK]
	Status: All 1 nodes are amd64 or arm64

========================================================================
SUMMARY
========================================================================
Total Checks: 6
OK:           3
Warning:      3
```

With `--verbose`, each check also shows its details, and the disk check lists every disk and why it
is or is not usable:

```console
[!!] Disks for OSDs [WARNING]
	Status: 2 usable disk(s) on 1 node(s)
	Details:
		- Ceph keeps 3 replicas on different hosts by default, so data stays degraded with disks on fewer nodes. For a test cluster, set the pool size to 1.
	Items:
		- lima-rook:/dev/vdc: 20.00 GB, usable
		- lima-rook:/dev/vdd: 20.00 GB, usable
		- lima-rook:/dev/vda: 100.00 GB, not usable: Has GPT headers, Has partitions
		- lima-rook:/dev/vdb: 20.00 GB, not usable: Has a FileSystem
```

Tainted nodes are not counted for mons and are not inspected, since the command does not know the
placement settings of the cluster.
