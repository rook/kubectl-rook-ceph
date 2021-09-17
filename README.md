<img alt="Rook" src="media/logo.svg" width="50%" height="50%">

# kubectl-rook-ceph

Provide common management and troubleshooting tools for the Rook Ceph storage provider as a `Krew` plugin.

## Install

TBD

## Commands
1. Command `kubectl rook get-mon-endpoints` is equivalent to getting `Data` of `rook-ceph-mon-endpoints`

```console
kubectl rook get-mon-endpoints
```
```console
Mon-endpoints
{
    "csi-cluster-config-json": "[{\"clusterID\":\"rook-ceph\",\"monitors\":[\"10.103.104.144:6789\"]}]",
    "data": "a=10.103.104.144:6789",
    "mapping": "{\"node\":{\"a\":{\"Name\":\"minikube\",\"Hostname\":\"minikube\",\"Address\":\"192.168.39.143\"}}}",
    "maxMonId": "0"
}
```

2. Command `kubectl rook ceph <args>` is equivalent to running `Ceph` commands inside `toolbox` pod(**NOTE:** all the Ceph Command is not tested).
```console
kubectl rook ceph <args>
```
+ > Example 1.

>```console
>kubectl rook ceph status
>```
>```console
>ceph status
>
> cluster:
>    id:     8924928d-c655-407e-8f2d-cb1e6ddad1d7
>    health: HEALTH_OK
>
>  services:
>    mon: 1 daemons, quorum a (age 2h)
>    mgr: a(active, since 2h)
>    osd: 1 osds: 1 up (since 2h), 1 in (since 2h)
>
>  data:
>    pools:   1 pools, 128 pgs
>    objects: 0 objects, 0 B
>    usage:   5.8 MiB used, 20 GiB / 20 GiB avail
>    pgs:     128 active+clean
>
>```

> Example 2
>```console
>kubectl rook ceph mon stat -f json-pretty
>```
>```console
>ceph mon stat --format json-pretty
>
>{
>    "epoch": 1,
>    "min_mon_release_name": "pacific",
>    "num_mons": 1,
>    "leader": "a",
>    "quorum": [
>        {
>            "rank": 0,
>            "name": "a"
>        }
>    ]
>}
>```

3. Command `kubectl rook operator restart` as name suggest it restart `ook-ceph-operator` pod.

```console
kubectl rook operator restart
```
```console
INFO[0000] Successfully restarted rook-ceph-operator pod
```

4. Command `kubectl rook operator set <property> <value>` is used to edit `rook-ceph-operator-config` configmap settings.
```console
kubectl rook operator set <property> <value>
```
>```console
>kubectl rook operator set ROOK_LOG_LEVEL DEBUG
>```
>```console
>Updated configmap rook-ceph-operator-config data
>
> {
>    "CSI_CEPHFS_FSGROUPPOLICY": "None",
>    "CSI_ENABLE_CEPHFS_SNAPSHOTTER": "true",
>    "CSI_ENABLE_RBD_SNAPSHOTTER": "true",
>    "CSI_ENABLE_VOLUME_REPLICATION": "false",
>    "CSI_FORCE_CEPHFS_KERNEL_CLIENT": "true",
>    "CSI_RBD_FSGROUPPOLICY": "ReadWriteOnceWithFSType",
>    "ROOK_CEPH_COMMANDS_TIMEOUT_SECONDS": "15",
>    "ROOK_CSI_ALLOW_UNSUPPORTED_VERSION": "false",
>    "ROOK_CSI_ENABLE_CEPHFS": "true",
>    "ROOK_CSI_ENABLE_GRPC_METRICS": "false",
>    "ROOK_CSI_ENABLE_RBD": "true",
>    "ROOK_ENABLE_DISCOVERY_DAEMON": "false",
>    "ROOK_ENABLE_FLEX_DRIVER": "false",
>    "ROOK_LOG_LEVEL": "DEBUG",
>    "ROOK_OBC_WATCH_OPERATOR_NAMESPACE": "true"
>}
>```


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
