# Mon Commands

## Print Mon Endpoints

This is used to print mon endpoints.

```bash
kubectl rook-ceph mons

# 10.98.95.196:6789,10.106.118.240:6789,10.111.18.121:6789
```

## Restore Quorum

Mon quorum is critical to the Ceph cluster. If majority of mons are not in quorum,
the cluster will be down. If the majority of mons are also lost permanently,
the quorum will need to be restore to a remaining good mon in order to bring
the Ceph cluster up again.

To restore the quorum in this disaster scenario:

1. Identify that mon quorum is lost. Some indications include:
   - The Rook operator log shows timeout errors and continuously fails to reconcile
   - All commands in the toolbox are unresponsive
   - Multiple mon pods are likely down
2. Identify which mon has good state.
   - Exec to a mon pod and run the following command
     - `ceph daemon mon.<name> mon_status`
     - For example, if connecting to mon.a, run: `ceph daemon mon.a mon_status`
   - If multiple mons respond, find the mon with the highest `election_epoch`
3. Start the toolbox pod if not already running
4. Run the command below to restore quorum to that good mon
5. Follow the prompts to confirm that you want to continue with each critical step of the restore
6. The final prompt will be to restart the operator, which will add new mons to restore the full quorum size

In this example, quorum is restored to mon **c**.

```bash
kubectl rook-ceph mons restore-quorum c
```

Before the restore proceeds, you will be prompted if you want to continue.
For example, here is the output in a test cluster up to the point of starting the restore.

```
$ kubectl rook-ceph mons restore-quorum c
mon=b, endpoint=192.168.64.168:6789
mon=c, endpoint=192.168.64.167:6789
mon=a, endpoint=192.168.64.169:6789
Info: Check for the running toolbox
Info: Waiting for the pod from deployment "rook-ceph-tools" to be running
deployment.apps/rook-ceph-tools condition met

Warning: Restoring mon quorum to mon c (192.168.64.167)
Info: The mons to discard are: b a
Info: Are you sure you want to restore the quorum to mon "c"? If so, enter: yes-really-restore
```

After entering `yes-really-restore`, the restore continues with output as such:

```
Info: proceeding
deployment.apps/rook-ceph-operator scaled
deployment.apps/rook-ceph-mon-a scaled
deployment.apps/rook-ceph-mon-b scaled
deployment.apps/rook-ceph-mon-c scaled
Info: Waiting for operator and mon pods to stop
pod/rook-ceph-operator-b5c96c99b-ggfg5 condition met
pod/rook-ceph-mon-a-6cd67d89b4-5qhgj condition met
pod/rook-ceph-mon-b-7d78cfd5-drztj condition met
pod/rook-ceph-mon-c-56499b77d7-sbmfb condition met
setting debug mode for "rook-ceph-mon-c"
Info: setting debug command to main container
Info: get pod for deployment rook-ceph-mon-c
deployment.apps/rook-ceph-mon-c-debug created
Info: ensure the debug deployment rook-ceph-mon-c is scaled up
deployment.apps/rook-ceph-mon-c-debug scaled
Info: Waiting for the pod from deployment "rook-ceph-mon-c-debug" to be running
deployment.apps/rook-ceph-mon-c-debug condition met
Info: Started debug pod, restoring the mon quorum in the debug pod
Info: Extracting the monmap

# Lengthy rocksdb output removed

Info: Printing final monmap
monmaptool: monmap file /tmp/monmap
epoch 3
fsid 818e8153-f17f-4ae3-b21f-078a4069affa
last_changed 2022-10-19T22:04:46.097595+0000
created 2022-10-19T22:01:11.347220+0000
min_mon_release 17 (quincy)
election_strategy: 1
0: [v2:192.168.64.167:3300/0,v1:192.168.64.167:6789/0] mon.c
Info: Restoring the mons in the rook-ceph-mon-endpoints configmap to the good mon
configmap/rook-ceph-mon-endpoints patched
Info: Stopping the debug pod for mon c
setting debug mode for "rook-ceph-mon-c-debug"
Info: removing debug mode from "rook-ceph-mon-c-debug"
deployment.apps "rook-ceph-mon-c-debug" deleted
deployment.apps/rook-ceph-mon-c scaled
Info: Check that the restored mon is responding
timed out
command terminated with exit code 1
Info: 0: waiting for ceph status to confirm single mon quorum
Info: sleeping 5
timed out
command terminated with exit code 1
Info: 1: waiting for ceph status to confirm single mon quorum
Info: sleeping 5
timed out
command terminated with exit code 1
Info: 2: waiting for ceph status to confirm single mon quorum
Info: sleeping 5
timed out
command terminated with exit code 1
Info: 3: waiting for ceph status to confirm single mon quorum
Info: sleeping 5

Info: finished waiting for ceph status
Info: Purging the bad mons: b a
Info: purging old mon: b
deployment.apps "rook-ceph-mon-b" deleted
Error from server (NotFound): services "rook-ceph-mon-b" not found
Info: purging mon pvc if exists
Error from server (NotFound): persistentvolumeclaims "rook-ceph-mon-b" not found
Info: purging old mon: a
deployment.apps "rook-ceph-mon-a" deleted
Error from server (NotFound): services "rook-ceph-mon-a" not found
Info: purging mon pvc if exists
Error from server (NotFound): persistentvolumeclaims "rook-ceph-mon-a" not found
Info: Mon quorum was successfully restored to mon c
Info: Only a single mon is currently running
Info: Press Enter to start the operator and expand to full mon quorum again
```

After reviewing that the cluster is healthy with a single mon, press Enter to continue:

```
Info: continuing
deployment.apps/rook-ceph-operator scaled
Info: The operator will now expand to full mon quorum
```
