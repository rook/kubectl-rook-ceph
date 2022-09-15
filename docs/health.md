# Health

Health command check health of the cluster and common configuration issues. Health command currently validates these things configurations (let us know if you would like to add other validation in health command):

1. at least three mon pods should running on different nodes
2. mon quorum and ceph health details
3. at least three osd pods should running on different nodes
4. all pods 'Running' status
5. placement group status
6. at least one mgr pod is running

Health commands logs have three ways of logging:

1. `Info`: This is just a logging information for the users.
2. `Warning`: which mean there is some improvement required in the cluster.
3. `Error`: This reuires immediate user attentions to get the cluster in healthy state.

## Output

```bash
kubectl rook-ceph health

# Info:  Checking if at least three mon pods are running on different nodes
# Warning:  At least three mon pods should running on different nodes
# rook-ceph-mon-a-5988949b9f-kfshx                1/1     Running       0          26s
# rook-ceph-mon-a-debug-6bc9d99979-4q2hd          1/1     Terminating   0          32s
# rook-ceph-mon-b-69c8cb6d85-vg6js                1/1     Running       0          2m29s
# rook-ceph-mon-c-6f6754bff5-746rp                1/1     Running       0          2m18s
#
# Info:  Checking mon quorum and ceph health details
# Warning:  HEALTH_WARN 1/3 mons down, quorum b,c
# [WRN] MON_DOWN: 1/3 mons down, quorum b,c
#     mon.a (rank 0) addr [v2:10.98.95.196:3300/0,v1:10.98.95.196:6789/0] is down (out of quorum)
#
# Info:  Checking if at least three osd pods are running on different nodes
# Warning:  At least three osd pods should running on different nodes
# rook-ceph-osd-0-debug-6f6f5496d8-m2nbp          1/1     Terminating   0          19s
#
# Info:  Pods that are in 'Running' status
# NAME                                            READY   STATUS        RESTARTS   AGE
# csi-cephfsplugin-provisioner-5f978bdb5b-7hbtr   5/5     Running       0          3m
# csi-cephfsplugin-vjl4c                          2/2     Running       0          3m
# csi-rbdplugin-cwkc2                             2/2     Running       0          3m
# csi-rbdplugin-provisioner-578f847bc-2j9ct       5/5     Running       0          3m
# rook-ceph-mgr-a-7b78b4b4b8-ndpmt                1/1     Running       0          2m7s
# rook-ceph-mon-a-5988949b9f-kfshx                1/1     Running       0          28s
# rook-ceph-mon-a-debug-6bc9d99979-4q2hd          1/1     Terminating   0          34s
# rook-ceph-mon-b-69c8cb6d85-vg6js                1/1     Running       0          2m31s
# rook-ceph-mon-c-6f6754bff5-746rp                1/1     Running       0          2m20s
# rook-ceph-operator-78cbdb59bd-4zcsh             1/1     Running       0          62s
# rook-ceph-osd-0-debug-6f6f5496d8-m2nbp          1/1     Terminating   0          19s
#
# Warning:  Pods that are 'Not' in 'Running' status
# NAME                                          READY   STATUS      RESTARTS   AGE
#
# Info:  checking placement group status
# Info:  2 pgs: 2 active+clean; 449 KiB data, 21 MiB used, 14 GiB / 14 GiB avail
#
# Info:  checking if at least one mgr pod is running
# rook-ceph-mgr-a-7b78b4b4b8-ndpmt                Running     fv-az290-487
```
