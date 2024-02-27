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

```bash
$ kubectl rook-ceph mons restore-quorum c

Info: mon c state is leader
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

```bash
Info: proceeding with resorting quorum
Info: Waiting for operator pod to stop
Info: rook-ceph-operator deployment scaled down
Info: Waiting for bad mon pod to stop
Info: deployment.apps/rook-ceph-mon-c scaled

Info: deployment.apps/rook-ceph-mon-a scaled

Info: fetching the deployment rook-ceph-mon-b to be running

Info: deployment rook-ceph-mon-b exists

Info: setting maintenance command to main container
Info: deployment rook-ceph-mon-b scaled down

Info: waiting for the deployment pod rook-ceph-mon-b-6cf85df489-l2lf4 to be deleted

Info: ensure the maintenance deployment rook-ceph-mon-b is scaled up

Info: waiting for pod with label "ceph_daemon_type=mon,ceph_daemon_id=b" in namespace "openshift-storage" to be running
Info: pod rook-ceph-mon-b-maintenance-5844bddbd7-k4cdk is ready for maintenance operations
Info: fetching the deployment rook-ceph-mon-b-maintenance to be running

Info: deployment rook-ceph-mon-b-maintenance exists

Info: Started maintenance pod, restoring the mon quorum in the maintenance pod
Info: Extracting the monmap


# Lengthy rocksdb output removed

Info: Finished updating the monmap!
Info: Printing final monmap
monmaptool: monmap file /tmp/monmap
epoch 3
fsid 4d32410e-fee1-4b0a-bc80-7f395fc43136
last_changed 2024-02-27T15:42:56.748095+0000
created 2024-02-27T15:42:03.020653+0000
min_mon_release 17 (quincy)
election_strategy: 1
0: v2:172.30.14.162:3300/0 mon.b
Info: Restoring the mons in the rook-ceph-mon-endpoints configmap to the good mon
Info: Stopping the maintenance pod for mon b.

Info: fetching the deployment rook-ceph-mon-b-maintenance to be running

Info: deployment rook-ceph-mon-b-maintenance exists

Info: removing maintenance mode from deployment rook-ceph-mon-b-maintenance

Info: Successfully deleted maintenance deployment and restored deployment "rook-ceph-mon-b"
Info: Check that the restored mon is responding
Error: failed to get the status of ceph cluster. failed to run command. failed to run command. command terminated with exit code 1
Info: 1: waiting for ceph status to confirm single mon quorum.

Info: current ceph status output

Info: sleeping for 5 seconds

Info: 10: waiting for ceph status to confirm single mon quorum.

Info: current ceph status output   cluster:
   id:     4d32410e-fee1-4b0a-bc80-7f395fc43136
   health: HEALTH_ERR
            1 filesystem is offline
            1 filesystem is online with fewer MDS than max_mds

   services:
     mon: 1 daemons, quorum b (age 93s)
     mgr: a(active, since 7m)
     mds: 0/0 daemons up
     osd: 3 osds: 3 up (since 6m), 3 in (since 7m)

   data:
     volumes: 1/1 healthy
     pools:   4 pools, 113 pgs
     objects: 9 objects, 580 KiB
     usage:   32 MiB used, 1.5 TiB / 1.5 TiB avail
     pgs:     113 active+clean



Info: sleeping for 5 seconds

Info: finished waiting for ceph status   cluster:
   id:     4d32410e-fee1-4b0a-bc80-7f395fc43136
   health: HEALTH_OK

   services:
     mon: 1 daemons, quorum b (age 101s)
     mgr: a(active, since 7m)
     mds: 1/1 daemons up
     osd: 3 osds: 3 up (since 6m), 3 in (since 7m)

   data:
     volumes: 1/1 healthy
     pools:   4 pools, 113 pgs
     objects: 31 objects, 582 KiB
     usage:   32 MiB used, 1.5 TiB / 1.5 TiB avail
     pgs:     113 active+clean

   io:
     client:   1.1 KiB/s wr, 0 op/s rd, 3 op/s wr

Info: Purging the bad mons [c a]

Info: purging bad mon: c

Info: purging bad mon: a

Info: Mon quorum was successfully restored to mon b

Info: Only a single mon is currently running
Info: Enter 'continue' to start the operator and expand to full mon quorum again
```

After reviewing that the cluster is healthy with a single mon, press Enter to continue:

```bash
continue
Info: proceeding with resorting quorum
```
