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
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/rook/kubectl-rook-ceph/pkg/logging"
)

const (
	bound    = "bound"
	orphaned = "orphaned"
)

func (f *CephFilesystem) SnapshotList(subvolgName, fsName string, orphanedOnly bool) {
	// Get snapshot IDs from Kubernetes VolumeSnapshotContent resources
	k8sSnapshotHandles := f.getK8sRefSnapshotHandle()

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "Filesystem\tSubvolume\tSubvolumeGroup\tSnapshot\tState")

	if err := f.filesystemExists(fsName); err != nil {
		logging.Error(err)
		return
	}

	if err := f.subvolumeGroupExists(fsName, subvolgName); err != nil {
		logging.Error(err)
		return
	}

	subvol, err := f.getSubvolumeNames(fsName, subvolgName)
	if err != nil {
		logging.Error(fmt.Errorf("failed to get subvolumes when listing snapshots for group %q in filesystem %q: %v", subvolgName, fsName, err))
		return
	}
	if len(subvol) == 0 {
		return
	}
	for _, sv := range subvol {
		cmd := "ceph"
		args := []string{"fs", "subvolume", "snapshot", "ls", fsName, sv.Name, subvolgName, "--format", "json"}
		snapList, err := f.runCommand(cmd, args)
		if err != nil {
			logging.Error(fmt.Errorf("failed to list snapshots of subvolume %q in group %q of filesystem %q: %v", sv.Name, subvolgName, fsName, err))
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

			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", fsName, sv.Name, subvolgName, snap.Name, status)
		}
	}
	w.Flush()
}

// SnapshotDelete deletes a CephFS snapshot after verifying it's orphaned.
func (f *CephFilesystem) SnapshotDelete(fs, subvol, snap, subvolgrp string) {
	// Get snapshot IDs from Kubernetes VolumeSnapshotContent resources to check if snapshot is orphaned
	k8sSnapshotHandles := f.getK8sRefSnapshotHandle()

	// Check if this snapshot exists in Kubernetes VolumeSnapshotContent
	// Extract the UUID from the csi-snap-<uuid> name to match the k8s snapshot handle IDs
	_, snapId := getSnapOmapVal(snap)
	_, isInK8s := k8sSnapshotHandles[snapId]

	if isInK8s {
		logging.Error(fmt.Errorf("snapshot %q is bound and cannot be deleted", snap))
		return
	}

	f.deleteSnapshot(fs, subvol, subvolgrp, snap)
	logging.Info("snapshot %q deleted successfully", snap)
}

func (f *CephFilesystem) getSubvolumeNames(fsName, svgName string) ([]fsStruct, error) {
	cmd := "ceph"
	args := []string{"fs", "subvolume", "ls", fsName, svgName, "--format", "json"}
	svList, err := f.runCommand(cmd, args)
	if err != nil {
		return nil, fmt.Errorf("failed to get subvolumes of %q: %w", fsName, err)
	}
	return unMarshaljson(svList), nil
}

func (f *CephFilesystem) filesystemExists(fsName string) error {
	cmd := "ceph"
	args := []string{"fs", "get", fsName, "--format", "json"}
	_, err := f.runCommand(cmd, args)
	if err != nil {
		return fmt.Errorf("filesystem %q does not exist: %w", fsName, err)
	}
	return nil
}

func (f *CephFilesystem) subvolumeGroupExists(fsName, svgName string) error {
	cmd := "ceph"
	args := []string{"fs", "subvolumegroup", "getpath", fsName, svgName}
	_, err := f.runCommand(cmd, args)
	if err != nil {
		return fmt.Errorf("subvolume group %q does not exist in filesystem %q: %w", svgName, fsName, err)
	}
	return nil
}
