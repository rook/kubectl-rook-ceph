# Rados

This used to run any rados cli command with with arbitrary args.

## Examples

```bash
kubectl rook-ceph rados df

# POOL_NAME     USED  OBJECTS  CLONES  COPIES  MISSING_ON_PRIMARY  UNFOUND  DEGRADED  RD_OPS      RD  WR_OPS       WR  USED COMPR  UNDER COMPR
# .mgr       452 KiB        2       0       2                   0        0         0      22  18 KiB      14  262 KiB         0 B          0 B

# total_objects    2
# total_used       27 MiB
# total_avail      10 GiB
# total_space      10 GiB
```

This also supports all the rados supported flags like `--format json-pretty`

```bash
kubectl rook-ceph rados df --format json-pretty

# {
#     "pools": [
#         {
#             "name": ".mgr",
#             "id": 1,
#             "size_bytes": 462848,
#             "size_kb": 452,
#             "num_objects": 2,
#             "num_object_clones": 0,
#             "num_object_copies": 2,
#             "num_objects_missing_on_primary": 0,
#             "num_objects_unfound": 0,
#             "num_objects_degraded": 0,
#             "read_ops": 22,
#             "read_bytes": 18432,
#             "write_ops": 14,
#             "write_bytes": 268288,
#             "compress_bytes_used": 0,
#             "compress_under_bytes": 0
#         }
#     ],
#     "total_objects": 2,
#     "total_used": 27404,
#     "total_avail": 10458356,
#     "total_space": 10485760
# }
```
