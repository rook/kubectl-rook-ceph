<img alt="Rook" src="media/logo.svg" width="50%" height="50%">

# kubectl-rook-ceph

Provide common management and troubleshooting tools for the [Rook Ceph](https://github.com/rook/rook) storage provider as a [Krew](https://github.com/kubernetes-sigs/krew) plugin.

## Install

To install the plugin, run:

  ```kubectl krew install rook-ceph```

## Usage

`kubectl rook-ceph <root-args> <command> <command-args>`

### Root args

- `--namespace` | `-n`: the Kubernetes namespace in which the CephCluster resides (default: rook-ceph)
- `--operator-namespace` | `-o`: the Kubernetes namespace in which the rook operator resides (default: rook-ceph)
- `--context`: the name of the Kubernetes context to be used
- `--help` | `-h`: Output help text

### Commands

- `ceph <args>` : Run a Ceph CLI command. Supports any arguments the `ceph` command supports. See [Ceph docs](https://docs.ceph.com/en/pacific/start/intro/) for more.

- `rbd <args>` : Call a 'rbd' CLI command with arbitrary args

- `mons` : Print mon endpoints

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

- `debug` : [Debug a deployment](#debug-mode)  by scaling it down and creating a debug copy. This is supported for mons and OSDs only
  - `start  <deployment-name> ` 
    `[--alternate-image <alternate-image>]` : Start debugging a deployment with an optional alternative ceph container image
  - `stop  <deployment-name>` : Stop debugging a deployment

- `help` : Output help text

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

### Debug Mode

Debug mode can be useful when a mon or OSD needs advanced maintenance operations that require the daemon to be stopped. Ceph tools such as `ceph-objectstore-tool`,`ceph-bluestore-tool`, or `ceph-monstore-tool` are commonly used in these scenarios. Debug mode will set up the mon or OSD so that these commands can be run.

Debug mode will automate the following:
1. Scale down the existing mon or OSD deployment
2. Start a new debug deployment where operations can be performed directly against the mon or OSD without that daemon running
   
   a. The main container sleeps so you can connect and run the ceph commands
   
   b. Liveness and startup probes are removed

   c. If alternate Image is passed by --alternate-image flag then the new debug deployment container will be using alternate Image.

For example, start the debug pod for mon `b`:
```console
kubectl rook-ceph debug start rook-ceph-mon-b
```
```text
setting debug mode for "rook-ceph-mon-b"
setting debug command to main container
deployment.apps/rook-ceph-mon-b scaled
deployment.apps/rook-ceph-mon-b-debug created
```

Now connect to the daemon pod and perform operations:
```console
kubectl exec <debug-pod> -- <ceph command>
```

When finished, stop debug mode and restore the original daemon:
```console
kubectl rook-ceph debug stop rook-ceph-mon-b
```
```text
setting debug mode for "rook-ceph-mon-b-debug"
removing debug mode from "rook-ceph-mon-b-debug"
deployment.apps "rook-ceph-mon-b-debug" deleted
deployment.apps/rook-ceph-mon-b scaled
```

>Note: If you need to update the limits and request of the debug deployment that is created using debug command you can run:
>```console
>oc set resources deployment rook-ceph-osd-${osdid}-debug --limits=cpu=8,memory=64Gi --requests=cpu=8,memory=64Gi
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
