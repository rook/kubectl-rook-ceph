# Operator

Operator is parent command which requires sub-command. Currently, operator supports these sub-commands:

1. `restart`: [restart](#restart) the Rook-Ceph operator
2. `set <property> <value>` : [set](#set) the property in the rook-ceph-operator-config configmap.

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
