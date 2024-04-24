# Ceph

This used to run any ceph cli command with with arbitrary args.

## Examples

```bash
kubectl rook-ceph ceph status

#   cluster:
#     id:     b74c18dd-6ee3-44fe-90b5-ed12feac46a4
#     health: HEALTH_OK
#
#   services:
#     mon: 3 daemons, quorum a,b,c (age 62s)
#     mgr: a(active, since 23s)
#     osd: 1 osds: 1 up (since 12s), 1 in (since 30s)
#
#   data:
#     pools:   0 pools, 0 pg
#     objects: 0 objects, 0 B
#     usage:   0 B used, 0 B / 0 B avail
#     pgs:
```

This also supports all the ceph supported flags like `--format json-pretty`

```bash
kubectl rook-ceph ceph status --format json-pretty

# {
#     "fsid": "b74c18dd-6ee3-44fe-90b5-ed12feac46a4",
#     "health": {
#         "status": "HEALTH_OK",
#         "checks": {},
#         "mutes": []
#     },
#     "election_epoch": 12,
#     "quorum": [
#         0,
#         1,
#         2
#     ],
#     "quorum_names": [
#         "a",
#         "b",
#         "c"
#     ],
#     "quorum_age": 67,
#     "monmap": {
#         "epoch": 3,
#         "min_mon_release_name": "quincy",
#         "num_mons": 3
#     },
#     "osdmap": {
#         "epoch": 13,
#         "num_osds": 1,
#         "num_up_osds": 1,
#         "osd_up_since": 1663145830,
#         "num_in_osds": 1,
#         "osd_in_since": 1663145812,
#         "num_remapped_pgs": 0
#     },
#     "pgmap": {
#         "pgs_by_state": [],
#         "num_pgs": 0,
#         "num_pools": 0,
#         "num_objects": 0,
#         "data_bytes": 0,
#         "bytes_used": 0,
#         "bytes_avail": 0,
#         "bytes_total": 0
#     },
#     "fsmap": {
#         "epoch": 1,
#         "by_rank": [],
#         "up:standby": 0
#     },
#     "mgrmap": {
#         "available": false,
#         "num_standbys": 0,
#         "modules": [
#             "dashboard",
#             "iostat",
#             "nfs",
#             "prometheus",
#             "restful"
#         ],
#         "services": {}
#     },
#     "servicemap": {
#         "epoch": 1,
#         "modified": "2022-09-14T08:55:39.603658+0000",
#         "services": {}
#     },
#     "progress_events": {}
# }
```

## Daemon

Run the Ceph daemon command by connecting to its admin socket.

```bash
kubectl rook-ceph ceph daemon osd.0 dump_historic_ops

Info: running 'ceph' command with args: [daemon osd.0 dump_historic_ops]
{
    "size": 20,
    "duration": 600,
    "ops": []
}
```
