# Logs

Print or stream the logs of Rook-Ceph pods, without needing to know the namespace they run in, the
labels Rook puts on them, or the names of their containers.

```bash
kubectl rook-ceph logs [target] [flags]
```

Exactly one of a target or `-l/--selector` is required. Passing both, or neither, is an error that
prints usage.

## Targets

A target names the pods to read. The namespaces are the ones already resolved by the plugin:
`operator` reads the operator namespace, and every other target reads the cluster namespace.

| Target | Namespace | Selector | Pods |
| --- | --- | --- | --- |
| `operator` | operator | `app=rook-ceph-operator` | The Rook operator |
| `exporter` | cluster | `app=rook-ceph-exporter` | The Ceph exporter on every node |
| `crashcollector` | cluster | `app=rook-ceph-crashcollector` | The crash collector on every node |
| `toolbox` | cluster | `app=rook-ceph-tools` | The Rook toolbox |
| `<type>` | cluster | `ceph_daemon_type=<type>` | Every Ceph daemon of that type |
| `<type>.<id>` | cluster | `ceph_daemon_type=<type>,ceph_daemon_id=<id>` | One Ceph daemon, such as `mon.a`, `osd.0`, or `mds.myfs-a` |
| `-l <selector>` | cluster | as given | Whatever the label selector matches |

`<type>` is one of the Ceph daemon types Rook labels its pods with: `mon`, `mgr`, `osd`, `mds`,
`rgw`, `nfs`, `nvmeof`, `rbd-mirror`, `fs-mirror`. Any other target is an error naming the valid
ones. The `<type>.<id>` form is the same daemon naming used by
`kubectl rook-ceph ceph daemon mon.a`.

```bash
kubectl rook-ceph logs operator --tail 5

# 2026-01-14 09:12:03.112841 I | ceph-cluster-controller: reconciling ceph cluster in namespace "rook-ceph"
# 2026-01-14 09:12:03.418922 I | op-mon: parsing mon endpoints: a=10.104.62.31:6789
# 2026-01-14 09:12:04.002317 I | ceph-cluster-controller: detecting the ceph image version for image quay.io/ceph/ceph:v18
# 2026-01-14 09:12:09.771208 I | ceph-cluster-controller: detected ceph image version: "18.2.1-0 reef"
# 2026-01-14 09:12:10.334019 I | ceph-cluster-controller: done reconciling ceph cluster in namespace "rook-ceph"
```

```bash
kubectl rook-ceph logs mon.a --tail 3

# debug 2026-01-14T09:11:58.512+0000 7f0e9d7f8700  0 log_channel(cluster) log [DBG] : monmap e3: 3 mons at {a=10.104.62.31:6789/0,b=10.98.51.7:6789/0,c=10.101.9.4:6789/0}
# debug 2026-01-14T09:12:01.884+0000 7f0e9b7f4700  1 mon.a@0(leader).osd e57 _set_new_cache_sizes cache_size:1020054731 inc_alloc: 71303168 full_alloc: 71303168 kv_alloc: 876609536
# debug 2026-01-14T09:12:03.902+0000 7f0e9d7f8700  0 log_channel(cluster) log [DBG] : osdmap e57: 3 total, 3 up, 3 in
```

## Flags

The following flags are supported. They behave as their `kubectl logs` counterparts do, except where
noted under [Differences from `kubectl logs`](#differences-from-kubectl-logs):

| Flag | Description |
| --- | --- |
| `-f`, `--follow` | Stream the logs until interrupted, reconnecting across restarts and picking up pods that appear later |
| `-p`, `--previous` | Show the logs of the previous container instance, for a container that has crashed |
| `--timestamps` | Prefix every line with a timestamp |
| `--tail <lines>` | Show only the most recent lines of each stream (default: all) |
| `--since <duration>` | Show only logs newer than a relative duration such as `5s`, `2m`, or `3h` |
| `-l`, `--selector <selector>` | Read the pods matching a label selector, instead of a target |
| `-c`, `--container <name>` | Read only the named container, which may be an init or ephemeral container |
| `--all-containers` | Read the init and ephemeral containers as well as the regular ones |
| `--max-log-requests <n>` | Maximum number of concurrent streams (default: 50) |

### Differences from `kubectl logs`

- `--max-log-requests` defaults to 50 rather than to the 5 of `kubectl logs`. A target such as `osd`
  is meant to match every OSD in the cluster, so a limit of 5 would reject an ordinary command.
- `--all-containers` adds the init and ephemeral containers to the regular ones. In `kubectl logs`
  it selects the regular containers along with the init and ephemeral ones; here the regular
  containers are already what a target fans out to, so only the other two are left to add. See
  [Init and ephemeral containers](#init-and-ephemeral-containers).
- `--tail` defaults to the whole retained log of every stream, where `kubectl logs -l` switches to
  the last 10 lines per pod as soon as a selector is used. A fan-out target run without `--tail`
  therefore prints every stream in full, which is worth avoiding on a target as large as `osd`.
- `--follow` reconnects across restarts and picks up pods that appear later, where `kubectl logs -f`
  ends when the container it attached to does. See [Following](#following).

## Reading several pods at once

A target such as `osd` or `mon` matches every pod of that type, and each pod contributes one stream
per container. The regular containers are read by default, `-c` narrows to a single container, and
`--all-containers` adds the init and ephemeral ones.

While exactly one stream is active the lines are printed unchanged. When a second stream starts,
every line gains a prefix, and prefixes stay on for the rest of the run — a rollout that briefly
doubles the pod count does not flap the output format back and forth. The prefix is `[pod]` while
every pod contributes exactly one container, so the common case is not padded with a redundant
container name:

```bash
kubectl rook-ceph logs mon --tail 1

# [rook-ceph-mon-a-7b9c4d6f5-8xk2p] debug 2026-01-14T09:12:03.902+0000 7f0e9d7f8700  0 log_channel(cluster) log [DBG] : osdmap e57: 3 total, 3 up, 3 in
# [rook-ceph-mon-b-5f4d97c88-t7wqd] debug 2026-01-14T09:12:03.911+0000 7f61b2c41700  1 mon.b@1(peon).osd e57 e57: 3 total, 3 up, 3 in
# [rook-ceph-mon-c-6cd7b4f9d-2zlmc] debug 2026-01-14T09:12:03.914+0000 7fa03d1e2700  1 mon.c@2(peon).osd e57 e57: 3 total, 3 up, 3 in
```

As soon as any pod contributes more than one container — an OSD with the log collector enabled, for
instance — the prefix widens to `[pod/container]` and stays that way, so the shape only ever grows
wider within a run:

```bash
kubectl rook-ceph logs osd.0 --tail 2

# [rook-ceph-osd-0-6c9d8f5b7d-4nq2z/osd] debug 2026-01-14T09:12:05.220+0000 7f2b6c1a4700  1 osd.0 57 tick checking mon for new map
# [rook-ceph-osd-0-6c9d8f5b7d-4nq2z/osd] debug 2026-01-14T09:12:06.884+0000 7f2b6c1a4700  1 osd.0 57 tick_without_osd_lock
# [rook-ceph-osd-0-6c9d8f5b7d-4nq2z/log-collector] + logrotate --verbose /etc/logrotate.d/ceph
# [rook-ceph-osd-0-6c9d8f5b7d-4nq2z/log-collector] + sleep 15m
```

Lines are ordered within a stream but not across streams, as with `kubectl logs -l`.

`--max-log-requests` caps how many streams may run at once, defaulting to 50. The default is high
enough that realistic clusters never reach it — a 12-OSD cluster with the log collector enabled is
24 streams — while a selector that matches hundreds of pods by mistake fails immediately instead of
opening hundreds of connections. Reaching the cap later, while following, only skips the pods above
it: ending a stream that is already running would lose the logs it was started to watch.

## Init and ephemeral containers

Only the regular containers of a pod are streamed by default. `-c` also accepts the name of an init
or an ephemeral container, and `--all-containers` streams the regular, init, and ephemeral
containers together.

This matters most for a daemon that never starts. A PVC-backed OSD prepares its device in init
containers such as `blkdevmapper`, `activate`, and `expand-bluefs`, so when the pod is stuck the
reason is in one of those logs rather than in the `osd` container, which has not run yet:

```bash
kubectl rook-ceph logs osd.0 -c activate --tail 20
```

An init container's log is finite. It ends where the container exited, so `--follow` has nothing
further to stream from it.

## Following

```bash
kubectl rook-ceph logs osd -f
```

`--follow` keeps streaming until it is interrupted. It reattaches when a container crashes or is
rolled onto a replacement pod, and it keeps looking for pods that match the target, so a daemon that
comes back under a new pod name is picked up and streamed as well. That is what makes `logs osd -f`
survive an OSD restart, which renames the pod. This differs from `kubectl logs -f`, which exits as
soon as the container it attached to does — the point being that a restart is usually the event
worth watching, not a reason to stop watching.

A replacement pod is read from the beginning of its log. A container that restarted in place resumes
one second before the last line already read, trading a repeated line or two for not losing lines to
clock skew between the client and the node.

A target that matches no pods is an error naming the selector and the namespace. Under `--follow`
that check applies only at startup: once streaming has begun, dropping to zero matching pods means
waiting for a replacement rather than exiting.

## Per-stream flags

`--tail`, `--since`, and `--previous` are passed to each stream individually, exactly as if a
separate `kubectl logs` had been run per pod and container. `--tail 5` against a 12-OSD cluster
therefore prints 60 lines, not 5, and `--since 5m` gives five minutes of history from every stream.

```bash
kubectl rook-ceph logs mds --tail 20 --timestamps
```

Read what a daemon logged before it crashed:

```bash
kubectl rook-ceph logs osd.3 -p
```

## Completing targets

Targets complete with the tab key. `logs <TAB>` offers the named components and the daemon types,
which are known without asking the cluster, and `logs osd.<TAB>` completes the OSDs that exist,
which is read from the cluster. The cluster lookup gives up after two seconds and completes nothing
rather than making the shell wait; a kubeconfig that authenticates with a credential plugin runs
that helper on every completion, which is not covered by those two seconds.

To complete `kubectl rook-ceph ...`, kubectl needs to know the plugin can complete for itself. Put
an executable named `kubectl_complete-rook_ceph` on `PATH` containing:

```bash
#!/usr/bin/env sh
exec kubectl rook-ceph __complete "$@"
```

Going back through `kubectl` rather than naming the binary is deliberate: krew installs the plugin
as `kubectl-rook_ceph`, so the binary's name depends on how it was installed while `kubectl
rook-ceph` does not.

To complete the binary when it is run directly instead, load the script it generates. The script
registers a command named `rook-ceph`, so a binary installed under any other name has to be bound to
it:

```bash
source <(kubectl-rook-ceph completion bash)
complete -o default -o nospace -F __start_rook-ceph kubectl-rook-ceph
```

## Pods the targets do not reach

The `toolbox` target selects `app=rook-ceph-tools`, which comes from Rook's example toolbox
manifest rather than from the operator. A toolbox deployed under different labels is not matched by
the target; use `-l` for it.

The bare `<type>` and `<type>.<id>` targets rely on the `ceph_daemon_type` and `ceph_daemon_id`
labels Rook applies to daemon pods. A pod created by a Rook release that predates those labels, and
not restarted since, does not carry them and is not matched. Read it by its `app` label instead:

```bash
kubectl rook-ceph logs -l app=rook-ceph-osd,ceph-osd-id=3 --tail 20
```

CSI pods are deployed by the separate ceph-csi-operator and have no target of their own. Select
them with `-l`, and use `-c` to pick the container. `-l` always reads the ceph cluster namespace,
while the CSI drivers run alongside the operator, so name the operator namespace with `-n` to reach
them:

```bash
kubectl rook-ceph logs -n <operator-namespace> -l app=rook-ceph.cephfs.csi.ceph.com-ctrlplugin -c csi-cephfsplugin --tail 20
```

In an installation where the operator and the cluster share a namespace, `-n` names that one
namespace and the command is the same.
