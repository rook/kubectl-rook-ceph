# Maintenance Mode

Maintenance mode can be useful when a mon or OSD needs advanced maintenance operations that require the daemon to be stopped. Ceph tools such as `ceph-objectstore-tool`,`ceph-bluestore-tool`, or `ceph-monstore-tool` are commonly used in these scenarios. Maintenance mode will set up the mon or OSD so that these commands can be run.

Maintenance mode will automate the following:

1. Scale down the existing mon or OSD deployment
2. Start a new maintenance deployment where operations can be performed directly against the mon or OSD without that daemon running
   a. The main container sleeps so you can connect and run the ceph commands
   b. Liveness and startup probes are removed
   c. If alternate Image is passed by --alternate-image flag then the new maintenance deployment container will be using alternate Image.

Maintenance mode provides these options:

1. [Start](#start-maintenance-mode) the maintenance deployment for troubleshooting.
2. [Stop](#stop-maintenance-mode) the temporary maintenance deployment
3. Update the resource limits for the deployment pod [advanced option](#advanced-options).

## Start maintenance mode

In this example we are using `mon-a` deployment

```bash
kubectl rook-ceph maintenance start rook-ceph-mon-a

Info: fetching the deployment rook-ceph-mon-a to be running

Info: deployment rook-ceph-mon-a exists

Info: setting maintenance command to main container
Info: deployment rook-ceph-mon-a scaled down

Info: waiting for the deployment pod rook-ceph-mon-a-df79bc6dc-2k8fc to be deleted

Info: ensure the maintenance deployment rook-ceph-mon-a is scaled up

Info: waiting for pod with label "ceph_daemon_type=mon,ceph_daemon_id=a" in namespace "rook-ceph" to be running
Info: pod rook-ceph-mon-a-maintenance-67b9f54944-jmcjj is ready for maintenance operations
```

Now connect to the daemon pod and perform operations:

```console
kubectl exec <maintenance-pod> -- <ceph command>
```

When finished, stop maintenance mode and restore the original daemon by running the command in the next section.

## Stop maintenance mode

Stop the deployment `mon-b` that is started above example.

```bash
kubectl rook-ceph maintenance stop rook-ceph-mon-b

Info: fetching the deployment rook-ceph-mon-a-maintenance to be running

Info: deployment rook-ceph-mon-a-maintenance exists

Info: removing maintenance mode from deployment rook-ceph-mon-a-maintenance

Info: Successfully deleted maintenance deployment and restored deployment "rook-ceph-mon-a"
```

## Advanced Options

If you need to update the limits and requests of the maintenance deployment that is created using maintenance command you can run:

>```console
>kubectl set resources deployment rook-ceph-osd-${osdid}-maintenance --limits=cpu=8,memory=64Gi --requests=cpu=8,memory=64Gi
>```
