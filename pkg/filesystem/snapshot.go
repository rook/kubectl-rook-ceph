/*
Copyright 2026 The Rook Authors. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package filesystem

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/rook/kubectl-rook-ceph/pkg/k8sutil"
	"github.com/rook/kubectl-rook-ceph/pkg/logging"
)

const (
	bound    = "bound"
	orphaned = "orphaned"
)

func SnapshotList(ctx context.Context, clientsets *k8sutil.Clientsets, operatorNamespace, clusterNamespace, subvolgName, fsName string, orphanedOnly bool) {
	// Get snapshot IDs from Kubernetes VolumeSnapshotContent resources
	k8sSnapshotHandles := getK8sRefSnapshotHandle(ctx, clientsets)

	fsstruct, err := getFileSystem(ctx, clientsets, operatorNamespace, clusterNamespace)
	if err != nil {
		logging.Error(err, "failed to get filesystem")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "Filesystem\tSubvolume\tSubvolumeGroup\tSnapshot\tState")

	for _, fs := range fsstruct {
		if fsName != "" && fs.Name != fsName {
			continue
		}
		subvolg, err := getSubvolumeGroup(ctx, clientsets, operatorNamespace, clusterNamespace, fs.Name)
		if err != nil {
			logging.Error(err, "failed to get subvolume groups")
			continue
		}
		for _, svg := range subvolg {
			if subvolgName != "" && svg.Name != subvolgName {
				continue
			}
			subvol, err := getSubvolumeNames(ctx, clientsets, operatorNamespace, clusterNamespace, fs.Name, svg.Name)
			if err != nil {
				logging.Error(err, "failed to get subvolumes for subvolume group %q", svg.Name)
				continue
			}
			if len(subvol) == 0 {
				continue
			}
			for _, sv := range subvol {
				cmd := "ceph"
				args := []string{"fs", "subvolume", "snapshot", "ls", fs.Name, sv.Name, svg.Name, "--format", "json"}
				snapList, err := runCommand(ctx, clientsets, operatorNamespace, clusterNamespace, cmd, args)
				if err != nil {
					logging.Error(err, "failed to get snapshots of %q", sv.Name)
					continue
				}
				snaps := unMarshaljson(snapList)
				for _, snap := range snaps {
					// Check if this snapshot exists in Kubernetes VolumeSnapshotContent
					// Extract the UUID from the csi-snap-<uuid> name to match the k8s snapshot handle IDs
					_, snapId := getSnapOmapVal(snap.Name)
					_, ok := k8sSnapshotHandles[snapId]

					status := bound
					if !ok {
						// Snapshot exists in Ceph but not in Kubernetes - it's orphaned
						status = orphaned
					}

					// If filtering for orphaned only, skip non-orphaned snapshots
					if orphanedOnly && status != orphaned {
						continue
					}

					fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", fs.Name, sv.Name, svg.Name, snap.Name, status)
				}
			}
		}
	}
	w.Flush()
}

// SnapshotDelete deletes a CephFS snapshot after verifying it's orphaned
func SnapshotDelete(ctx context.Context, clientsets *k8sutil.Clientsets, operatorNamespace, clusterNamespace, fs, subvol, snap, subvolgrp string) {
	// Get snapshot IDs from Kubernetes VolumeSnapshotContent resources to check if snapshot is orphaned
	k8sSnapshotHandles := getK8sRefSnapshotHandle(ctx, clientsets)

	// Check if this snapshot exists in Kubernetes VolumeSnapshotContent
	// Extract the UUID from the csi-snap-<uuid> name to match the k8s snapshot handle IDs
	_, snapId := getSnapOmapVal(snap)
	_, isInK8s := k8sSnapshotHandles[snapId]

	if isInK8s {
		logging.Error(fmt.Errorf("snapshot %q is bound and cannot be deleted", snap))
		return
	}

	deleteSnapshot(ctx, clientsets, operatorNamespace, clusterNamespace, fs, subvol, subvolgrp, snap)
	logging.Info("snapshot %q deleted successfully", snap)
}

func getSubvolumeNames(ctx context.Context, clientsets *k8sutil.Clientsets, operatorNamespace, clusterNamespace, fsName, svgName string) ([]fsStruct, error) {
	cmd := "ceph"
	args := []string{"fs", "subvolume", "ls", fsName, svgName, "--format", "json"}
	svList, err := runCommand(ctx, clientsets, operatorNamespace, clusterNamespace, cmd, args)
	if err != nil {
		return nil, fmt.Errorf("failed to get subvolumes of %q: %w", fsName, err)
	}
	return unMarshaljson(svList), nil
}
