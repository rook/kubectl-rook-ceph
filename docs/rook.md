# Rook

The `rook` command supports the following sub-commands:

1. `purge-osd <osd-id> [--force]` : [purge osd](#purge-an-osd) permanently remove an OSD from the cluster. Multiple OSDs can be removed in a single command with a comma-separated list of IDs.
2. `version`: [version](#version) prints the rook version.
3. `status`: [status](#status) print the phase and conditions of the CephCluster CR
4. `status all`: [status all](#status-all) print the phase and conditions of all CRs
5. `status <CR>`: [status  cr](#status-cr-name) print the phase and conditions of CRs of a specific type, such as 'cephobjectstore', 'cephfilesystem', etc

## Purge OSDs

Permanently remove OSD(s) from the cluster.

!!! warning
    Data loss is possible when passing the --force flag if the PGs are not healthy on other OSDs.

```bash
kubectl rook-ceph rook purge-osd 0 --force

# 2022-09-14 08:58:28.888431 I | rookcmd: starting Rook v1.10.0-alpha.0.164.gcb73f728c with arguments 'rook ceph osd remove --osd-ids=0 --force-osd-removal=true'
# 2022-09-14 08:58:28.889217 I | rookcmd: flag values: --force-osd-removal=true, --help=false, --log-level=INFO, --operator-image=, --osd-ids=0, --preserve-pvc=false, --service-account=
# 2022-09-14 08:58:28.889582 I | op-mon: parsing mon endpoints: b=10.106.118.240:6789
# 2022-09-14 08:58:28.898898 I | cephclient: writing config file /var/lib/rook/rook-ceph/rook-ceph.config
# 2022-09-14 08:58:28.899567 I | cephclient: generated admin config in /var/lib/rook/rook-ceph
# 2022-09-14 08:58:29.421345 I | cephosd: validating status of osd.0
# 2022-09-14 08:58:29.421371 I | cephosd: osd.0 is healthy. It cannot be removed unless it is 'down'
```

Multiple OSDs can be removed in one invocation with a comma-separated list of IDs.

## Version

Print the version of Rook.

```bash
kubectl rook-ceph rook version

#rook: v1.10.0-alpha.0.164.gcb73f728c
#go: go1.18.5
```

## Status

```bash
kubectl rook-ceph rook status

# Info: cephclusters my-cluster
# ceph:
#   capacity:
#     bytesAvailable: 20942778368
#     bytesTotal: 20971520000
#     bytesUsed: 28741632
#     lastUpdated: "2024-01-11T06:43:27Z"
#   fsid: 5a8dc97a-1d21-4c0e-aac2-fdc924e18374
#   health: HEALTH_OK
#   lastChanged: "2024-01-11T05:38:27Z"
#   lastChecked: "2024-01-11T06:43:27Z"
#   previousHealth: HEALTH_ERR
#   versions:
#     mgr:
#       ceph version 18.2.1 (7fe91d5d5842e04be3b4f514d6dd990c54b29c76) reef (stable): 1
#     mon:
#       ceph version 18.2.1 (7fe91d5d5842e04be3b4f514d6dd990c54b29c76) reef (stable): 1
#     osd:
#       ceph version 18.2.1 (7fe91d5d5842e04be3b4f514d6dd990c54b29c76) reef (stable): 1
#     overall:
#       ceph version 18.2.1 (7fe91d5d5842e04be3b4f514d6dd990c54b29c76) reef (stable): 3
# conditions:
# - lastHeartbeatTime: "2024-01-11T05:37:46Z"
#   lastTransitionTime: "2024-01-11T05:37:46Z"
#   message: Processing OSD 0 on node "minikube"
#   reason: ClusterProgressing
#   status: "True"
#   type: Progressing
# - lastHeartbeatTime: "2024-01-11T06:43:27Z"
#   lastTransitionTime: "2024-01-11T05:38:27Z"
#   message: Cluster created successfully
#   reason: ClusterCreated
#   status: "True"
#   type: Ready
# message: Cluster created successfully
# observedGeneration: 2
# phase: Ready
# state: Created
# storage:
#   deviceClasses:
#   - name: hdd
#   osd:
#     storeType:
#       bluestore: 1
# version:
#   image: quay.io/ceph/ceph:v18
#   version: 18.2.1-0
```

## Status All

```bash
kubectl rook-ceph rook status all

# Info: cephclusters my-cluster
# ceph:
#   capacity:
#     bytesAvailable: 20942770176
#     bytesTotal: 20971520000
#     bytesUsed: 28749824
#     lastUpdated: "2024-01-11T06:44:28Z"
#   details:
#     POOL_APP_NOT_ENABLED:
#       message: 1 pool(s) do not have an application enabled
#       severity: HEALTH_WARN
#   fsid: 5a8dc97a-1d21-4c0e-aac2-fdc924e18374
#   health: HEALTH_WARN
#   lastChanged: "2024-01-11T06:44:28Z"
#   lastChecked: "2024-01-11T06:44:28Z"
#   previousHealth: HEALTH_OK
#   versions:
#     mds:
#       ceph version 18.2.1 (7fe91d5d5842e04be3b4f514d6dd990c54b29c76) reef (stable): 2
#     mgr:
#       ceph version 18.2.1 (7fe91d5d5842e04be3b4f514d6dd990c54b29c76) reef (stable): 1
#     mon:
#       ceph version 18.2.1 (7fe91d5d5842e04be3b4f514d6dd990c54b29c76) reef (stable): 1
#     osd:
#       ceph version 18.2.1 (7fe91d5d5842e04be3b4f514d6dd990c54b29c76) reef (stable): 1
#     overall:
#       ceph version 18.2.1 (7fe91d5d5842e04be3b4f514d6dd990c54b29c76) reef (stable): 5
# conditions:
# - lastHeartbeatTime: "2024-01-11T05:37:46Z"
#   lastTransitionTime: "2024-01-11T05:37:46Z"
#   message: Processing OSD 0 on node "minikube"
#   reason: ClusterProgressing
#   status: "True"
#   type: Progressing
# - lastHeartbeatTime: "2024-01-11T06:44:28Z"
#   lastTransitionTime: "2024-01-11T05:38:27Z"
#   message: Cluster created successfully
#   reason: ClusterCreated
#   status: "True"
#   type: Ready
# message: Cluster created successfully
# observedGeneration: 2
# phase: Ready
# state: Created
# storage:
#   deviceClasses:
#   - name: hdd
#   osd:
#     storeType:
#       bluestore: 1
# version:
#   image: quay.io/ceph/ceph:v18
#   version: 18.2.1-0

# Info: resource cephblockpoolradosnamespaces was not found on the cluster
# Info: cephblockpools builtin-mgr
# observedGeneration: 2
# phase: Ready

# Info: resource cephbucketnotifications was not found on the cluster
# Info: resource cephbuckettopics was not found on the cluster
# Info: resource cephclients was not found on the cluster
# Info: resource cephcosidrivers was not found on the cluster
# Info: resource cephfilesystemmirrors was not found on the cluster
# Info: cephfilesystems myfs
# observedGeneration: 2
# phase: Ready

# Info: cephfilesystemsubvolumegroups myfs-csi
# info:
#   clusterID: e1026845ad66577abae1d16671b464c8
# observedGeneration: 1
# phase: Ready

# Info: resource cephnfses was not found on the cluster
# Info: resource cephobjectrealms was not found on the cluster
# Info: resource cephobjectstores was not found on the cluster
# Info: resource cephobjectstoreusers was not found on the cluster
# Info: resource cephobjectzonegroups was not found on the cluster
# Info: resource cephobjectzones was not found on the cluster
# Info: resource cephrbdmirrors was not found on the cluster
```

## Status `<cr-name>`

```bash
kubectl rook-ceph rook status cephclusters

# Info: cephclusters my-cluster
# ceph:
#   capacity:
#     bytesAvailable: 20942778368
#     bytesTotal: 20971520000
#     bytesUsed: 28741632
#     lastUpdated: "2024-01-11T06:43:27Z"
#   fsid: 5a8dc97a-1d21-4c0e-aac2-fdc924e18374
#   health: HEALTH_OK
#   lastChanged: "2024-01-11T05:38:27Z"
#   lastChecked: "2024-01-11T06:43:27Z"
#   previousHealth: HEALTH_ERR
#   versions:
#     mgr:
#       ceph version 18.2.1 (7fe91d5d5842e04be3b4f514d6dd990c54b29c76) reef (stable): 1
#     mon:
#       ceph version 18.2.1 (7fe91d5d5842e04be3b4f514d6dd990c54b29c76) reef (stable): 1
#     osd:
#       ceph version 18.2.1 (7fe91d5d5842e04be3b4f514d6dd990c54b29c76) reef (stable): 1
#     overall:
#       ceph version 18.2.1 (7fe91d5d5842e04be3b4f514d6dd990c54b29c76) reef (stable): 3
# conditions:
# - lastHeartbeatTime: "2024-01-11T05:37:46Z"
#   lastTransitionTime: "2024-01-11T05:37:46Z"
#   message: Processing OSD 0 on node "minikube"
#   reason: ClusterProgressing
#   status: "True"
#   type: Progressing
# - lastHeartbeatTime: "2024-01-11T06:43:27Z"
#   lastTransitionTime: "2024-01-11T05:38:27Z"
#   message: Cluster created successfully
#   reason: ClusterCreated
#   status: "True"
#   type: Ready
# message: Cluster created successfully
# observedGeneration: 2
# phase: Ready
# state: Created
# storage:
#   deviceClasses:
#   - name: hdd
#   osd:
#     storeType:
#       bluestore: 1
# version:
#   image: quay.io/ceph/ceph:v18
#   version: 18.2.1-0
```
