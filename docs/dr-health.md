# Disaster Recovery

## Health

The DR health command is used to get the connection status of one cluster from another cluster in mirroring-enabled clusters. The cephblockpool is queried with mirroring-enabled and If not found will exit with relevant logs. Optionally, args can be passed to be appended to `ceph status`, for example: `--debug-ms 1`.

Example: `kubectl rook-ceph dr health [ceph status args]`

```bash
kubectl rook-ceph dr health

# Info: running ceph status from other cluster details
#   cluster:
#     id:     32261f66-1qw2e-48bb-ae4e-d8de080714ca
#     health: HEALTH_WARN
#             clock skew detected on mon.b, mon.c

#   services:
#     mon:        3 daemons, quorum a,b,c (age 2h)
#     mgr:        a(active, since 2h)
#     mds:        1/1 daemons up, 1 hot standby
#     osd:        3 osds: 3 up (since 2h), 3 in (since 8d)
#     rbd-mirror: 1 daemon active (1 hosts)
#     rgw:        1 daemon active (1 hosts, 1 zones)

#   data:
#     volumes: 1/1 healthy
#     pools:   11 pools, 321 pgs
#     objects: 931 objects, 1.4 GiB
#     usage:   4.3 GiB used, 1.5 TiB / 1.5 TiB avail
#     pgs:     321 active+clean

#   io:
#     client:   45 KiB/s rd, 156 KiB/s wr, 55 op/s rd, 21 op/s wr

# Info: running mirroring daemon health
# health: OK
# daemon health: OK
# image health: OK
# images: 4 total
#     4 replaying
```
