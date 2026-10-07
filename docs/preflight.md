# Preflight

The `preflight` command checks whether a Kubernetes cluster's nodes can run a Rook Ceph cluster,
before the cluster is created, and says which example to start from when the nodes are too few for
the defaults of the example `cluster.yaml`.

Unlike other commands, `preflight` does not need Rook to be installed.

It validates the following:

1. **Kubernetes Version**: the server version is in the range that Rook supports.
2. **Node Architecture**: every node is `amd64` or `arm64`.
3. **Mon Placement**: there are at least three schedulable nodes, so the three mons of the example
   `cluster.yaml` can run on different nodes. Nodes that are not ready, are cordoned, or have a
   `NoSchedule` or `NoExecute` taint are not counted.
4. **Kernel**: the kernel is 5.11 or newer, which the kernel RBD and CephFS clients need to connect
   with msgr2, the default, and the `rbd` module is loaded, built in, or installed.
5. **Disks for OSDs**: the disks on each node that Rook can use for OSDs, with the reason each other
   disk is skipped. It warns when fewer than three nodes have disks, because pools with the default
   replica size need three hosts.
6. **Node Memory**: each node's allocatable memory covers its OSDs (4GiB each, the default
   `osd_memory_target`) and a mon. This is an estimate.

Checks 4 to 6 inspect the nodes with a short-lived privileged pod on each schedulable node. The pod
uses the Ceph image and runs `ceph-volume inventory`, the same inventory that the OSD prepare job
uses to decide which disks it can use. If the pods cannot be created or do not finish, the checks
that need them report an error. Pass `--skip-node-inspection` to run only checks 1 to 4 without
any pods. The pods are deleted when the command finishes.

The command exits with status 1 if any check is critical or fails, so it can be used in scripts.

## Usage

```bash
kubectl rook-ceph preflight
kubectl rook-ceph preflight -o json
kubectl rook-ceph preflight --skip-node-inspection
```

Flags:

- `--image`: Ceph image for the node inspection pods. Defaults to the image in the example
    manifests.
- `--probe-namespace`: namespace for the node inspection pods. Defaults to the cluster namespace
    (`-n`, `rook-ceph` by default) if it exists, otherwise `default`. The namespace must allow
    privileged pods.
- `--node-selector`: label selector for the nodes to check.
- `--skip-node-inspection`: do not create pods, and skip the disk, `rbd` and memory checks.
- `-o`, `--output`: `text` (default), `json` or `yaml`.
- `--verbose`: list every node and disk.

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
	Status: 2 usable disk(s) on 1 node(s); pools with the default replica size need disks on 3 nodes
	Details:
		- Pools in the example manifests keep 3 replicas on different hosts and stop serving I/O with fewer. For a test cluster, create pools from pool-test.yaml, filesystem-test.yaml and object-test.yaml, which keep one replica, or set the pool size to 1.

[OK] Kernel [OK]
	Status: Kernel versions support msgr2 and the rbd module is available

========================================================================
K8s Resources
========================================================================

[!!] Mon Placement [WARNING]
	Status: Only 1 node(s) are schedulable; the 3 mons of the example cluster.yaml need 3 nodes
	Details:
		- Nodes that are not ready, cordoned, or have a NoSchedule or NoExecute taint are not counted. Rook placement settings can tolerate taints.
		- For a single-node test cluster, start from cluster-test.yaml, which runs one mon.

[!!] Node Memory [WARNING]
	Status: 1 node(s) may not have enough memory for their OSDs
	Details:
		- lima-rook: 7.6Gi allocatable, about 9.0Gi needed for 2 OSD(s) and a mon
		- The estimate uses the default osd_memory_target of 4GiB per OSD. OSDs can run with less by lowering osd_memory_target, at some cost in performance.

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

With `--verbose`, the disk check lists every disk and why it is or is not usable:

```console
	Items:
		- lima-rook:/dev/vdc: 20.00 GB, usable
		- lima-rook:/dev/vdd: 20.00 GB, usable
		- lima-rook:/dev/vda: 100.00 GB, not usable: Has GPT headers, Has partitions
		- lima-rook:/dev/vdb: 20.00 GB, not usable: Has a FileSystem
```

## Limitations

- Placement settings, such as node affinity and tolerations, are not evaluated. Tainted nodes are
    not counted for mons and are not inspected, even if the CephCluster will tolerate the taint.
