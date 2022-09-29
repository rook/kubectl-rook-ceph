# Debug Mode

Debug mode can be useful when a mon or OSD needs advanced maintenance operations that require the daemon to be stopped. Ceph tools such as `ceph-objectstore-tool`,`ceph-bluestore-tool`, or `ceph-monstore-tool` are commonly used in these scenarios. Debug mode will set up the mon or OSD so that these commands can be run.

Debug mode will automate the following:

1. Scale down the existing mon or OSD deployment
2. Start a new debug deployment where operations can be performed directly against the mon or OSD without that daemon running
   a. The main container sleeps so you can connect and run the ceph commands
   b. Liveness and startup probes are removed
   c. If alternate Image is passed by --alternate-image flag then the new debug deployment container will be using alternate Image.

Debug mode provides these options:

1. [Start](#start-debug-mode) the debug deployment for troubleshooting.
2. [Stop](#stop-debug-mode) the temporary debug deployment
3. Update the resource limits for the deployment pod [advanced option](#advanced-options).

## Start debug mode

In this example we are using `mon-b` deployment

```bash
kubectl rook-ceph debug start rook-ceph-mon-b

# setting debug mode for "rook-ceph-mon-b"
# setting debug command to main container
# deployment.apps/rook-ceph-mon-b scaled
# deployment.apps/rook-ceph-mon-b-debug created
```

Now connect to the daemon pod and perform operations:

```console
kubectl exec <debug-pod> -- <ceph command>
```

When finished, stop debug mode and restore the original daemon by running the command in the next section.

## Stop debug mode

Stop the deployment `mon-b` that is started above example.

```bash
kubectl rook-ceph debug stop rook-ceph-mon-b

# setting debug mode for "rook-ceph-mon-b-debug"
# removing debug mode from "rook-ceph-mon-b-debug"
# deployment.apps "rook-ceph-mon-b-debug" deleted
# deployment.apps/rook-ceph-mon-b scaled
```

## Advanced Options

If you need to update the limits and requests of the debug deployment that is created using debug command you can run:

>```console
>kubectl set resources deployment rook-ceph-osd-${osdid}-debug --limits=cpu=8,memory=64Gi --requests=cpu=8,memory=64Gi
>```
