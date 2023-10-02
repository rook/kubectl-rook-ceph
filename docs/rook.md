# Rook

The `rook` command supports the following sub-commands:

1. `purge-osd <osd-id> [--force]` : [purge osd](#purge-osd) permanently remove an OSD from the cluster. Multiple OSDs can be removed in a single command with a comma-separated list of IDs.
2. `version`: [version](#version) prints the rook version.
3. `status`      : [status](#status) print the phase and conditions of the CephCluster CR
4. `status all`  : [status all](#status-all) print the phase and conditions of all CRs
5. `status <CR>` : [status  cr](#status-cr-name) print the phase and conditions of CRs of a specific type, such as 'cephobjectstore', 'cephfilesystem', etc

## Purge an OSD

Permanently remove an OSD from the cluster. 


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

# "ceph": {
#     "capacity": {},
#     "fsid": "b74c18dd-6ee3-44fe-90b5-ed12feac46a4",
#     "health": "HEALTH_OK",
#     "lastChecked": "2022-09-14T08:57:00Z",
#     "versions": {
#       "mgr": {
#         "ceph version 17.2.3 (dff484dfc9e19a9819f375586300b3b79d80034d) quincy (stable)": 1
#       },
#       "mon": {
#         "ceph version 17.2.3 (dff484dfc9e19a9819f375586300b3b79d80034d) quincy (stable)": 3
#       },
#       "overall": {
#         "ceph version 17.2.3 (dff484dfc9e19a9819f375586300b3b79d80034d) quincy (stable)": 4
#       }
#     }
#   },
#   "conditions": [
#     {
#       "lastHeartbeatTime": "2022-09-14T08:57:02Z",
#       "lastTransitionTime": "2022-09-14T08:56:59Z",
#       "message": "Cluster created successfully",
#       "reason": "ClusterCreated",
#       "status": "True",
#       "type": "Ready"
#     },
#     {
#       "lastHeartbeatTime": "2022-09-14T08:57:30Z",
#       "lastTransitionTime": "2022-09-14T08:57:30Z",
#       "message": "Detecting Ceph version",
#       "reason": "ClusterProgressing",
#       "status": "True",
#       "type": "Progressing"
#     }
#   ],
#   "message": "Detecting Ceph version",
#   "observedGeneration": 2,
#   "phase": "Progressing",
#   "state": "Creating",
#   "version": {
#     "image": "quay.io/ceph/ceph:v17",
#     "version": "17.2.3-0"
#   }
```

## All Status

```bash
kubectl rook-ceph rook status all

# cephblockpoolradosnamespaces.ceph.rook.io:
# cephblockpools.ceph.rook.io: {
#   "observedGeneration": 2,
#   "phase": "Failure"
# }
# {
#   "phase": "Progressing"
# }
# cephbucketnotifications.ceph.rook.io:
# cephbuckettopics.ceph.rook.io:
# cephclients.ceph.rook.io:
# cephclusters.ceph.rook.io: {
#   "ceph": {
#     "capacity": {},
#     "fsid": "b74c18dd-6ee3-44fe-90b5-ed12feac46a4",
#     "health": "HEALTH_OK",
#     "lastChecked": "2022-09-14T08:57:00Z",
#     "versions": {
#       "mgr": {
#         "ceph version 17.2.3 (dff484dfc9e19a9819f375586300b3b79d80034d) quincy (stable)": 1
#       },
#       "mon": {
#         "ceph version 17.2.3 (dff484dfc9e19a9819f375586300b3b79d80034d) quincy (stable)": 3
#       },
#       "overall": {
#         "ceph version 17.2.3 (dff484dfc9e19a9819f375586300b3b79d80034d) quincy (stable)": 4
#       }
#     }
#   },
#   "conditions": [
#     {
#       "lastHeartbeatTime": "2022-09-14T08:57:02Z",
#       "lastTransitionTime": "2022-09-14T08:56:59Z",
#       "message": "Cluster created successfully",
#       "reason": "ClusterCreated",
#       "status": "True",
#       "type": "Ready"
#     },
#     {
#       "lastHeartbeatTime": "2022-09-14T08:57:37Z",
#       "lastTransitionTime": "2022-09-14T08:57:37Z",
#       "message": "Configuring Ceph Mons",
#       "reason": "ClusterProgressing",
#       "status": "True",
#       "type": "Progressing"
#     }
#   ],
#   "message": "Configuring Ceph Mons",
#   "observedGeneration": 2,
#   "phase": "Progressing",
#   "state": "Creating",
#   "version": {
#     "image": "quay.io/ceph/ceph:v17",
#     "version": "17.2.3-0"
#   }
# }
# cephfilesystemmirrors.ceph.rook.io:
# cephfilesystems.ceph.rook.io:
# cephfilesystemsubvolumegroups.ceph.rook.io:
# cephnfses.ceph.rook.io:
# cephobjectrealms.ceph.rook.io:
# cephobjectstores.ceph.rook.io:
# cephobjectstoreusers.ceph.rook.io:
# cephobjectzonegroups.ceph.rook.io:
# cephobjectzones.ceph.rook.io:
# cephrbdmirrors.ceph.rook.io:
# objectbucketclaims.objectbucket.io:
```

## Status `<cr-name>`

```bash
kubectl rook-ceph rook status cephclusters

# cephclusters.ceph.rook.io: {
#   "ceph": {
#     "capacity": {},
#     "fsid": "b74c18dd-6ee3-44fe-90b5-ed12feac46a4",
#     "health": "HEALTH_OK",
#     "lastChecked": "2022-09-14T08:57:00Z",
#     "versions": {
#       "mgr": {
#         "ceph version 17.2.3 (dff484dfc9e19a9819f375586300b3b79d80034d) quincy (stable)": 1
#       },
#       "mon": {
#         "ceph version 17.2.3 (dff484dfc9e19a9819f375586300b3b79d80034d) quincy (stable)": 3
#       },
#       "overall": {
#         "ceph version 17.2.3 (dff484dfc9e19a9819f375586300b3b79d80034d) quincy (stable)": 4
#       }
#     }
#   },
#   "conditions": [
#     {
#       "lastHeartbeatTime": "2022-09-14T08:57:02Z",
#       "lastTransitionTime": "2022-09-14T08:56:59Z",
#       "message": "Cluster created successfully",
#       "reason": "ClusterCreated",
#       "status": "True",
#       "type": "Ready"
#     },
#     {
#       "lastHeartbeatTime": "2022-09-14T08:57:37Z",
#       "lastTransitionTime": "2022-09-14T08:57:37Z",
#       "message": "Configuring Ceph Mons",
#       "reason": "ClusterProgressing",
#       "status": "True",
#       "type": "Progressing"
#     }
#   ],
#   "message": "Configuring Ceph Mons",
#   "observedGeneration": 2,
#   "phase": "Progressing",
#   "state": "Creating",
#   "version": {
#     "image": "quay.io/ceph/ceph:v17",
#     "version": "17.2.3-0"
#   }
# }
```
