# Toolbox

The `toolbox` command opens an interactive Bash shell in a running Rook Ceph toolbox pod. It finds the pod using the `app=rook-ceph-tools` label selector.

The toolbox pod must already be deployed in the Ceph cluster namespace, and the current Kubernetes user must have permission to list pods and execute commands in them.

## Usage

```bash
kubectl rook-ceph toolbox
```

To use a toolbox in a different namespace:

```bash
kubectl rook-ceph --namespace my-rook-ceph-namespace toolbox
```

Run `exit` to leave the toolbox shell.
