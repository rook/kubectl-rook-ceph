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
	"strings"

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
	svg string
	fs  string
}

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
		if pv.Spec.CSI != nil && pv.Spec.CSI.VolumeAttributes["subvolumeName"] != "" {
			subvolumeNames[pv.Spec.CSI.VolumeAttributes["subvolumeName"]] = subVolumeInfo{}
		}
	}
	return subvolumeNames
}

// listCephFSSubvolumes list all the subvolumes
func listCephFSSubvolumes(ctx context.Context, clientsets *k8sutil.Clientsets, operatorNamespace, clusterNamespace string, includeStaleOnly bool, subvolumeNames map[string]subVolumeInfo) {

	// getFilesystem gets the filesystem
	fsstruct := getFileSystem(ctx, clientsets, operatorNamespace, clusterNamespace)
	var svList string
	fmt.Println("Filesystem  Subvolume  SubvolumeGroup  State")

	// this iterates over the filesystems and subvolumegroup to get the list of subvolumes that exist
	for _, fs := range fsstruct {
		// gets the subvolumegroup in the filesystem
		subvolg := getSubvolumeGroup(ctx, clientsets, operatorNamespace, clusterNamespace, fs.Name)
		for _, svg := range subvolg {
			svList = exec.RunCommandInOperatorPod(ctx, clientsets, "ceph", []string{"fs", "subvolume", "ls", fs.Name, svg.Name}, operatorNamespace, clusterNamespace, true, false)
			subvol := unMarshaljson(svList)
			if len(subvol) == 0 {
				continue
			}
			// append the subvolume which doesn't have any snapshot attached to it.
			for _, sv := range subvol {
				// Assume the volume is stale unless proven otherwise
				stale := true
				// lookup for subvolume in list of the PV references
				_, ok := subvolumeNames[sv.Name]
				if ok || checkSnapshot(ctx, clientsets, operatorNamespace, clusterNamespace, fs.Name, sv.Name, svg.Name) {
					// The volume is not stale if a PV was found, or it has a snapshot
					stale = false
				}
				status := "stale"
				if !stale {
					if includeStaleOnly {
						continue
					}
					status = "in-use"
				}
				subvolumeNames[sv.Name] = subVolumeInfo{fs.Name, svg.Name}
				fmt.Println(fs.Name, sv.Name, svg.Name, status)
			}
		}
	}
}

// gets list of filesystem
func getFileSystem(ctx context.Context, clientsets *k8sutil.Clientsets, operatorNamespace, clusterNamespace string) []fsStruct {
	fsList := exec.RunCommandInOperatorPod(ctx, clientsets, "ceph", []string{"fs", "ls", "--format", "json"}, operatorNamespace, clusterNamespace, true, false)
	fsstruct := unMarshaljson(fsList)
	if len(fsstruct) == 0 {
		logging.Fatal(fmt.Errorf("failed to get filesystem"))
	}
	return fsstruct
}

// checkSnapshot checks if there are any snapshots in the subvolume
func checkSnapshot(ctx context.Context, clientsets *k8sutil.Clientsets, operatorNamespace, clusterNamespace, fs, sv, svg string) bool {

	snapList := exec.RunCommandInOperatorPod(ctx, clientsets, "ceph", []string{"fs", "subvolume", "snapshot", "ls", fs, sv, svg}, operatorNamespace, clusterNamespace, true, false)
	snap := unMarshaljson(snapList)
	if len(snap) == 0 {
		return false
	}
	return true

}

// gets the list of subvolumegroup for the specified filesystem
func getSubvolumeGroup(ctx context.Context, clientsets *k8sutil.Clientsets, operatorNamespace, clusterNamespace, fs string) []fsStruct {
	svgList := exec.RunCommandInOperatorPod(ctx, clientsets, "ceph", []string{"fs", "subvolumegroup", "ls", fs, "--format", "json"}, operatorNamespace, clusterNamespace, true, false)
	subvolg := unMarshaljson(svgList)
	if len(subvolg) == 0 {
		logging.Fatal(fmt.Errorf("failed to get subvolumegroup for filesystem %q", fs))
	}
	return subvolg
}

func unMarshaljson(list string) []fsStruct {
	var unmarshal []fsStruct
	errg := json.Unmarshal([]byte(list), &unmarshal)
	if errg != nil {
		logging.Fatal(errg)
	}

	return unmarshal
}

func Delete(ctx context.Context, clientsets *k8sutil.Clientsets, OperatorNamespace, CephClusterNamespace, subList, fs, svg string) {
	subvollist := strings.Split(subList, ",")
	k8sSubvolume := getK8sRefSubvolume(ctx, clientsets)
	for _, subvolume := range subvollist {
		check := checkStaleSubvolume(ctx, clientsets, OperatorNamespace, CephClusterNamespace, fs, subvolume, svg, k8sSubvolume)
		if check {
			exec.RunCommandInOperatorPod(ctx, clientsets, "ceph", []string{"fs", "subvolume", "rm", fs, subvolume, svg}, OperatorNamespace, CephClusterNamespace, true, false)
			logging.Info("subvolume %q deleted", subvolume)
		} else {
			logging.Info("subvolume %q is not stale", subvolume)
		}
	}
}

// checkStaleSubvolume checks if there are any stale subvolume to be deleted
func checkStaleSubvolume(ctx context.Context, clientsets *k8sutil.Clientsets, OperatorNamespace, CephClusterNamespace, fs, subvolume, svg string, k8sSubvolume map[string]subVolumeInfo) bool {
	_, ok := k8sSubvolume[subvolume]
	if !ok {
		snapshot := checkSnapshot(ctx, clientsets, OperatorNamespace, CephClusterNamespace, fs, subvolume, svg)
		if snapshot {
			logging.Error(fmt.Errorf("subvolume %s has snapshots", subvolume))
			return false
		} else {
			return true
		}
	}
	logging.Error(fmt.Errorf("Subvolume %s is referenced by a PV", subvolume))
	return false
}
