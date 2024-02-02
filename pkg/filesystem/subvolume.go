/*
Copyright 2023 The Rook Authors. All rights reserved.

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

package subvolume

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/rook/kubectl-rook-ceph/pkg/exec"
	"github.com/rook/kubectl-rook-ceph/pkg/k8sutil"
	"github.com/rook/kubectl-rook-ceph/pkg/logging"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type fsStruct struct {
	Name string
	data []string
}

type subVolumeInfo struct {
	svg   string
	fs    string
	state string
}

const (
	inUse             = "in-use"
	stale             = "stale"
	staleWithSnapshot = "stale-with-snapshot"
)

func List(ctx context.Context, clientsets *k8sutil.Clientsets, operatorNamespace, clusterNamespace string, includeStaleOnly bool) {

	subvolumeNames := getK8sRefSubvolume(ctx, clientsets)
	listCephFSSubvolumes(ctx, clientsets, operatorNamespace, clusterNamespace, includeStaleOnly, subvolumeNames)
}

// getk8sRefSubvolume returns the k8s ref for the subvolumes
func getK8sRefSubvolume(ctx context.Context, clientsets *k8sutil.Clientsets) map[string]subVolumeInfo {
	pvList, err := clientsets.Kube.CoreV1().PersistentVolumes().List(ctx, v1.ListOptions{})
	if err != nil {
		logging.Fatal(fmt.Errorf("Error fetching PVs: %v\n", err))
	}
	subvolumeNames := make(map[string]subVolumeInfo)
	for _, pv := range pvList.Items {
		if pv.Spec.CSI != nil {
			subvolumeNames[pv.Spec.CSI.VolumeAttributes["subvolumeName"]] = subVolumeInfo{}
		}
	}
	return subvolumeNames
}

// listCephFSSubvolumes list all the subvolumes
func listCephFSSubvolumes(ctx context.Context, clientsets *k8sutil.Clientsets, operatorNamespace, clusterNamespace string, includeStaleOnly bool, subvolumeNames map[string]subVolumeInfo) {

	// getFilesystem gets the filesystem
	fsstruct, err := getFileSystem(ctx, clientsets, operatorNamespace, clusterNamespace)
	if err != nil {
		logging.Error(err, "failed to get filesystem")
		return
	}
	fmt.Println("Filesystem  Subvolume  SubvolumeGroup  State")

	// this iterates over the filesystems and subvolumegroup to get the list of subvolumes that exist
	for _, fs := range fsstruct {
		// gets the subvolumegroup in the filesystem
		subvolg, err := getSubvolumeGroup(ctx, clientsets, operatorNamespace, clusterNamespace, fs.Name)
		if err != nil {
			logging.Error(err, "failed to get subvolume groups")
			continue
		}
		for _, svg := range subvolg {
			svList, err := exec.RunCommandInOperatorPod(ctx, clientsets, "ceph", []string{"fs", "subvolume", "ls", fs.Name, svg.Name}, operatorNamespace, clusterNamespace, true)
			if err != nil {
				logging.Error(err, "failed to get subvolumes of %q", fs.Name)
				continue
			}
			subvol := unMarshaljson(svList)
			if len(subvol) == 0 {
				continue
			}
			// append the subvolume which doesn't have any snapshot attached to it.
			for _, sv := range subvol {
				state := getSubvolumeState(ctx, clientsets, operatorNamespace, clusterNamespace, fs.Name, sv.Name, svg.Name)

				// Assume the volume is stale unless proven otherwise
				stalevol := true
				// lookup for subvolume in list of the PV references
				_, ok := subvolumeNames[sv.Name]
				if ok {
					// The volume is not stale if a PV was found
					stalevol = false
				}
				status := stale
				if !stalevol {
					if includeStaleOnly {
						continue
					}
					status = inUse
				} else {
					// check the state of the stale subvolume
					// if it is snapshot-retained then skip listing it.
					if state == "snapshot-retained" {
						status = state
						continue
					}
					// check if the stale subvolume has snapshots.
					if checkSnapshot(ctx, clientsets, operatorNamespace, clusterNamespace, fs.Name, sv.Name, svg.Name) {
						status = staleWithSnapshot
					}

				}
				subvolumeNames[sv.Name] = subVolumeInfo{fs.Name, svg.Name, state}
				fmt.Println(fs.Name, sv.Name, svg.Name, status)
			}
		}
	}
}

// getSubvolumeState returns the state of the subvolume
func getSubvolumeState(ctx context.Context, clientsets *k8sutil.Clientsets, operatorNamespace, clusterNamespace, fsName, SubVol, SubvolumeGroup string) string {
	subVolumeInfo, errvol := exec.RunCommandInOperatorPod(ctx, clientsets, "ceph", []string{"fs", "subvolume", "info", fsName, SubVol, SubvolumeGroup}, operatorNamespace, clusterNamespace, true)
	if errvol != nil {
		logging.Error(errvol, "failed to get filesystems")
		return ""
	}
	var info map[string]interface{}
	err := json.Unmarshal([]byte(subVolumeInfo), &info)
	if err != nil {
		logging.Fatal(fmt.Errorf("failed to unmarshal: %q", err))
	}
	state, ok := info["state"].(string)
	if !ok {
		logging.Fatal(fmt.Errorf("failed to get the state of subvolume: %q", SubVol))
	}
	return state
}

// gets list of filesystem
func getFileSystem(ctx context.Context, clientsets *k8sutil.Clientsets, operatorNamespace, clusterNamespace string) ([]fsStruct, error) {
	fsList, err := exec.RunCommandInOperatorPod(ctx, clientsets, "ceph", []string{"fs", "ls", "--format", "json"}, operatorNamespace, clusterNamespace, true)
	if err != nil {
		logging.Error(err, "failed to get filesystems")
		return []fsStruct{}, err
	}
	fsstruct := unMarshaljson(fsList)
	if len(fsstruct) == 0 {
		logging.Fatal(fmt.Errorf("no filesystem found"))
	}
	return []fsStruct{}, nil
}

// checkSnapshot checks if there are any snapshots in the subvolume
func checkSnapshot(ctx context.Context, clientsets *k8sutil.Clientsets, operatorNamespace, clusterNamespace, fs, sv, svg string) bool {

	snapList, err := exec.RunCommandInOperatorPod(ctx, clientsets, "ceph", []string{"fs", "subvolume", "snapshot", "ls", fs, sv, svg}, operatorNamespace, clusterNamespace, true)
	if err != nil {
		logging.Error(err, "failed to get subvolume snapshots of %q/%q/%q", fs, sv, svg)
		return false
	}
	snap := unMarshaljson(snapList)
	if len(snap) == 0 {
		return false
	}
	return true

}

// gets the list of subvolumegroup for the specified filesystem
func getSubvolumeGroup(ctx context.Context, clientsets *k8sutil.Clientsets, operatorNamespace, clusterNamespace, fs string) ([]fsStruct, error) {
	svgList, err := exec.RunCommandInOperatorPod(ctx, clientsets, "ceph", []string{"fs", "subvolumegroup", "ls", fs, "--format", "json"}, operatorNamespace, clusterNamespace, true)
	if err != nil {
		logging.Error(err, "failed to get subvolume groups for filesystem %q", fs)
		return []fsStruct{}, err
	}
	subvolg := unMarshaljson(svgList)
	if len(subvolg) == 0 {
		logging.Fatal(fmt.Errorf("no subvolumegroups found for filesystem %q", fs))
	}
	return subvolg, nil
}

func unMarshaljson(list string) []fsStruct {
	var unmarshal []fsStruct
	errg := json.Unmarshal([]byte(list), &unmarshal)
	if errg != nil {
		logging.Fatal(errg)
	}

	return unmarshal
}

func Delete(ctx context.Context, clientsets *k8sutil.Clientsets, OperatorNamespace, CephClusterNamespace, fs, subvol, svg string) {
	k8sSubvolume := getK8sRefSubvolume(ctx, clientsets)
	_, check := k8sSubvolume[subvol]
	if !check {
		exec.RunCommandInOperatorPod(ctx, clientsets, "ceph", []string{"fs", "subvolume", "rm", fs, subvol, svg, "--retain-snapshots"}, OperatorNamespace, CephClusterNamespace, true)
		logging.Info("subvolume %q deleted", subvol)
	} else {
		logging.Info("subvolume %q is not stale", subvol)
	}
}
