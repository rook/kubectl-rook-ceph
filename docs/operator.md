# Operator

Operator is parent command which requires sub-command. Currently, operator supports these sub-commands:

1. `restart`: [restart](#restart) the Rook-Ceph operator
2. `set <property> <value>` : [set](#set) the property in the rook-ceph-operator-config configmap.
3. `logs`: [logs](#logs) print the logs of the rook-ceph operator pod

## Restart

Restart the Rook-Ceph operator.

```bash
kubectl rook-ceph operator restart

# deployment.apps/rook-ceph-operator restarted
```

## Set

Set the property in the rook-ceph-operator-config configmap

```bash
kubectl rook-ceph operator set ROOK_LOG_LEVEL DEBUG

# configmap/rook-ceph-operator-config patched
```

## Logs

Print the logs of the rook-ceph operator pod, without needing to know its namespace or label. The
operator namespace is the one already resolved by the plugin.

The operator runs as a single replica, so the command reads from a single pod: a running one when
several match, as happens briefly during a restart. When more than one pod matches, it reports which
pod it chose.

```bash
kubectl rook-ceph operator logs --tail 5

# 2026-01-14 09:12:03.112841 I | ceph-cluster-controller: reconciling ceph cluster in namespace "rook-ceph"
# 2026-01-14 09:12:03.418922 I | op-mon: parsing mon endpoints: a=10.104.62.31:6789
# 2026-01-14 09:12:04.002317 I | ceph-cluster-controller: detecting the ceph image version for image quay.io/ceph/ceph:v18
# 2026-01-14 09:12:09.771208 I | ceph-cluster-controller: detected ceph image version: "18.2.1-0 reef"
# 2026-01-14 09:12:10.334019 I | ceph-cluster-controller: done reconciling ceph cluster in namespace "rook-ceph"
```

The following flags are supported, all matching their `kubectl logs` counterparts:

| Flag | Description |
| --- | --- |
| `-f`, `--follow` | Stream the logs until interrupted, reconnecting across operator restarts |
| `--tail <lines>` | Show only the most recent lines (default: all) |
| `-p`, `--previous` | Show the logs of the previous container instance, for an operator that has crashed |
| `--since <duration>` | Show only logs newer than a relative duration such as `5s`, `2m`, or `3h` |
| `--timestamps` | Prefix every line with a timestamp |

Stream the logs of a running operator:

```bash
kubectl rook-ceph operator logs -f
```

`--follow` keeps streaming until it is interrupted, reattaching when the operator container crashes
or is rolled onto a replacement pod. This differs from `kubectl logs -f`, which exits as soon as the
container it attached to does — the point being that an operator restart is usually the event worth
watching, not a reason to stop watching. A replacement pod is read from the beginning of its log; a
container that restarted in place resumes from where the previous stream left off, which may repeat
a line or two.

Read the logs an operator emitted before it crashed:

```bash
kubectl rook-ceph operator logs -p
```
