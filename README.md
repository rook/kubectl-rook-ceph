<img alt="Rook" src="media/logo.svg" width="50%" height="50%">

# kubectl-rook-ceph

Provide common management and troubleshooting tools for the [Rook Ceph](https://github.com/rook/rook) storage provider as a [Krew](https://github.com/kubernetes-sigs/krew) plugin.

## Install

> Note: This requires kubectl [krew](https://krew.sigs.k8s.io/docs/user-guide/setup/install/) to be installed.

To install the plugin, run:

  ```kubectl krew install rook-ceph```

To check plugin version `kubectl krew list` this will list all krew plugin with their current version.

## Update

  ```kubectl krew upgrade rook-ceph```

## Usage

`kubectl rook-ceph <root-args> <command> <command-args>`

### Root args

These are args currently supported:

1. `-h|--help`: this will print brief command help text.

    ```bash
    kubectl rook-ceph --help
    ```

2. `-n|--namespace='rook-ceph'`: the Kubernetes namespace in which the CephCluster resides. (optional,  default: rook-ceph)

    ```bash
    kubectl rook-ceph -o test-operator -n test-cluster rook version
    ```

3. `-o|--operator-namespace` : the Kubernetes namespace in which the rook operator resides, when the arg `-n` is passed but `-o` is not then `-o` will equal to the `-n`. (default: rook-ceph)

    ```bash
    kubectl rook-ceph -o test-operator -n test-cluster rook version
    ```

4. `--context`: the name of the Kubernetes context to be used (optional).

    ```bash
    kubectl rook-ceph --context=$(kubectl config current-context) mons
    ```

### Commands

- `ceph <args>` : Run a Ceph CLI command. Supports any arguments the `ceph` command supports. See [Ceph docs](https://docs.ceph.com/en/pacific/start/intro/) for more.

- `ceph daemon <daemon.id> <args>`: Run a Ceph daemon command by connecting to its admin socket.

- `rados <args>` : Run a Rados CLI command. Supports any arguments the `rados` command supports. See [Rados docs](https://docs.ceph.com/en/latest/man/8/rados/) for more.

- `radosgw-admin <args>` : Run an RGW CLI command. Supports any arguments the `radosgw-admin` command supports. See the [radosgw-admin docs](https://docs.ceph.com/en/latest/man/8/radosgw-admin/) for more.

- `rbd <args>` : Call a 'rbd' CLI command with arbitrary args

- `mons` : Print mon endpoints
  - `restore-quorum <mon-name>` : Restore the mon quorum based on a single healthy mon since quorum was lost with the other mons

- `health` : check health of the cluster and common configuration issues

- `operator`
  - `restart` : Restart the Rook-Ceph operator
  - `set <property> <value>` : Set the property in the rook-ceph-operator-config configmap.

- `rook`
  - `version`     : Print the version of Rook
  - `status`      : Print the phase and/or conditions of every CR in the namespace
  - `status all`  : Print the phase and conditions of all CRs
  - `status <CR>` : Print the phase and conditions of CRs of a specific type, such as `cephobjectstore`, `cephfilesystem`, etc
  - `purge-osd <osd-id> [--force]` : Permanently remove an OSD from the cluster. Multiple OSDs can be removed with a comma-separated list of IDs.

- `maintenance` : [Perform maintenance operations](docs/maintenance.md) on mons or OSDs. The mon or OSD deployment will be scaled down and replaced temporarily by a maintenance deployment.
  - `start  <deployment-name>`
    `[--alternate-image <alternate-image>]` : Start a maintenance deployment with an optional alternative ceph container image
  - `stop  <deployment-name>` : Stop the maintenance deployment and restore the mon or OSD deployment

- `dr` :
  - `health [ceph status args]`: Print the `ceph status` of a peer cluster in a mirroring-enabled environment thereby validating connectivity between ceph clusters. Ceph status args can be optionally passed, such as to change the log level: `--debug-ms 1`.

- `restore-deleted <CRD> [CRName]`: Restore the ceph resources which are stuck in deleting state due to underlying resources being present in the cluster

- `multus validation` : Validate whether the current Multus and system configurations will support Rook with Multus. See [Validating Multus configuration](https://rook.github.io/docs/rook/latest/CRDs/Cluster/network-providers/?h=valid#validating-multus-configuration) for more details.
  - `run` : Run Multus validation tests to verify network connectivity
  - `config` : Generate a sample validation test config file
  - `cleanup` : Clean up Multus validation test resources

- `help` : Output help text

## Documentation

Visit docs below for complete details about each command and their flags uses.

1. [Running ceph commands](docs/ceph.md)
1. [Running ceph daemon commands](docs/ceph.md#Daemon)
1. [Running rbd commands](docs/rbd.md)
1. [Get mon endpoints](docs/mons.md#print-mon-endpoints)
1. [Get cluster health status](docs/health.md)
1. [Update configmap rook-ceph-operator-config](docs/operator.md#set)
1. [Restart operator pod](docs/operator.md#restart)
1. [Get rook version](docs/rook.md#version)
1. [Get all CR status](docs/rook.md#status-all)
1. [Get cephCluster CR status](docs/rook.md#status)
1. [Get specific CR status](docs/rook.md#status-cr-name)
1. [To purge OSD](docs/rook.md#operator.md)
1. [Perform maintenance for OSDs and Mons](docs/maintenance.md)
1. [Restore mon quorum](docs/mons.md#restore-quorum)
1. [Disaster Recovery](docs/dr-health.md)
1. [Restore deleted CRs](docs/crd.md)
1. [Destroy cluster](docs/destroy-cluster.md)
1. [Running rados commands](docs/rados.md)
1. [Multus validation](docs/multus.md)

## Examples

### Run a Ceph Command

Any `ceph` command can be run with the plugin. This example gets the ceph status:

```console
kubectl rook-ceph ceph status
```

>```text
>  cluster:
>    id:     a1ac6554-4cc8-4c3b-a8a3-f17f5ec6f529
>    health: HEALTH_OK
>
>  services:
>    mon: 3 daemons, quorum a,b,c (age 11m)
>    mgr: a(active, since 10m)
>    mds: 1/1 daemons up, 1 hot standby
>    osd: 3 osds: 3 up (since 10m), 3 in (since 8d)
>
>  data:
>    volumes: 1/1 healthy
>    pools:   6 pools, 137 pgs
>    objects: 34 objects, 4.1 KiB
>    usage:   58 MiB used, 59 GiB / 59 GiB avail
>    pgs:     137 active+clean
>
>  io:
>    client:   1.2 KiB/s rd, 2 op/s rd, 0 op/s wr
>```

### Restart the Operator

```console
kubectl rook-ceph operator restart
```

>```text
>deployment.apps/rook-ceph-operator restarted
>```

### Rook Version

```console
kubectl rook-ceph rook version
```

```text
rook: v1.8.0-alpha.0.267.g096dabfa6
go: go1.16.13
```

### Ceph Versions

```console
kubectl rook-ceph ceph versions
```

```text
{
    "mon": {
        "ceph version 16.2.7 (dd0603118f56ab514f133c8d2e3adfc983942503) pacific (stable)": 1
    },
    "mgr": {
        "ceph version 16.2.7 (dd0603118f56ab514f133c8d2e3adfc983942503) pacific (stable)": 1
    },
    "osd": {
        "ceph version 16.2.7 (dd0603118f56ab514f133c8d2e3adfc983942503) pacific (stable)": 1
    },
    "mds": {},
    "overall": {
        "ceph version 16.2.7 (dd0603118f56ab514f133c8d2e3adfc983942503) pacific (stable)": 3
    }
}
```

## Contributing

We welcome contributions. See the [Rook Contributing Guide](https://github.com/rook/rook/blob/master/CONTRIBUTING.md) to get started.

## Report a Bug

For filing bugs, suggesting improvements, or requesting new features, please open an [issue](https://github.com/rook/kubectl-rook-ceph/issues).

## Contact

Please use the following to reach members of the community:

- Slack: Join our [slack channel](https://slack.rook.io)
- Forums: [rook-dev](https://groups.google.com/forum/#!forum/rook-dev)
- Twitter: [@rook_io](https://twitter.com/rook_io)
- Email (general topics): [cncf-rook-info@lists.cncf.io](mailto:cncf-rook-info@lists.cncf.io)
- Email (security topics): [cncf-rook-security@lists.cncf.io](mailto:cncf-rook-security@lists.cncf.io)

## Licensing

Rook is under the Apache 2.0 license.

[![FOSSA Status](https://app.fossa.io/api/projects/git%2Bgithub.com%2Frook%2Frook.svg?type=large)](https://app.fossa.io/projects/git%2Bgithub.com%2Frook%2Frook?ref=badge_large)
