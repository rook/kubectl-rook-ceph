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

In this example, quorum is restore to mon **a**.

```bash
kubectl rook-ceph mons restore-quorum a
```
