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
	Name         string
	MetadataName string `json:"metadata_pool"`
	data         []string
}

type subVolumeInfo struct {
	svg   string
	fs    string
	state string
}

type monitor struct {
	ClusterID string
	Monitors  []string
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

// checkForExternalStorage checks if the external mode is enabled.
func checkForExternalStorage(ctx context.Context, clientsets *k8sutil.Clientsets, clusterNamespace string) bool {
	enable := false
	cephclusters, err := clientsets.Rook.CephV1().CephClusters(clusterNamespace).List(ctx, v1.ListOptions{})
	if err != nil {
		logging.Fatal(fmt.Errorf("failed to list CephClusters in namespace %q: %v", clusterNamespace, err))
	}
	for i := range cephclusters.Items {
		enable = cephclusters.Items[i].Spec.External.Enable
		if enable == true {
			break
		}
	}
	return enable
}

// getExternalClusterDetails gets the required mon-ip, id and key to connect to the
// ceph cluster.
func getExternalClusterDetails(ctx context.Context, clientsets *k8sutil.Clientsets, operatorNamespace, clusterNamespace string) (string, string, string) {
	var adminId, adminKey, m string

	scList, err := clientsets.Kube.CoreV1().Secrets(clusterNamespace).List(ctx, v1.ListOptions{})
	if err != nil {
		logging.Fatal(fmt.Errorf("Error fetching secrets in namespace %q: %v", clusterNamespace, err))
	}
	for i := range scList.Items {
		if strings.HasPrefix(scList.Items[i].ObjectMeta.Name, "rook-csi-cephfs-provisioner") {
			data := scList.Items[i].Data
			if data == nil {
				logging.Fatal(fmt.Errorf("Secret data is empty for %s/%s", clusterNamespace, scList.Items[i].ObjectMeta.Name))
			}
			adminId = string(data["adminID"])
			adminKey = string(data["adminKey"])
			break

		}
	}

	cm, err := clientsets.Kube.CoreV1().ConfigMaps(clusterNamespace).Get(ctx, "rook-ceph-mon-endpoints", v1.GetOptions{})
	if err != nil {
		logging.Fatal(fmt.Errorf("Error fetching configmaps %s/rook-ceph-mon-endpoints: %v", clusterNamespace, err))
	}

	if len(cm.Data) == 0 || cm.Data == nil {
		logging.Fatal(fmt.Errorf("Configmap data is empty for %s/rook-ceph-mon-endpoints", clusterNamespace))
	}
	monpoint := cm.Data["csi-cluster-config-json"]
	var monip []monitor
	json.Unmarshal([]byte(monpoint), &monip)
	for _, mp := range monip {
		if len(mp.Monitors) == 0 || mp.Monitors[0] == "" {
			logging.Fatal(fmt.Errorf("mon ip for %s/rook-ceph-mon-endpoints with clusterID:%q is empty", clusterNamespace, mp.ClusterID))
		}
		m = mp.Monitors[0]
	}

	return m, adminId, adminKey
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

// runCommand checks for the presence of externalcluster and runs the command accordingly.
func runCommand(ctx context.Context, clientsets *k8sutil.Clientsets, operatorNamespace, clusterNamespace, cmd string, args []string) (string, error) {
	if checkForExternalStorage(ctx, clientsets, clusterNamespace) {
		m, admin_id, admin_key := getExternalClusterDetails(ctx, clientsets, operatorNamespace, clusterNamespace)
		args = append(args, "-m", m, "--id", admin_id, "--key", admin_key)
	}
	list, err := exec.RunCommandInOperatorPod(ctx, clientsets, cmd, args, operatorNamespace, clusterNamespace, true)

	return list, err
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
			cmd := "ceph"
			args := []string{"fs", "subvolume", "ls", fs.Name, svg.Name, "--format", "json"}
			svList, err := runCommand(ctx, clientsets, operatorNamespace, clusterNamespace, cmd, args)
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
	cmd := "ceph"
	args := []string{"fs", "subvolume", "info", fsName, SubVol, SubvolumeGroup, "--format", "json"}

	subVolumeInfo, errvol := runCommand(ctx, clientsets, operatorNamespace, clusterNamespace, cmd, args)
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

	cmd := "ceph"
	args := []string{"fs", "ls", "--format", "json"}

	fsList, err := runCommand(ctx, clientsets, operatorNamespace, clusterNamespace, cmd, args)
	if err != nil {
		logging.Error(err, "failed to get filesystems")
		return []fsStruct{}, err
	}
	fsstruct := unMarshaljson(fsList)
	if len(fsstruct) == 0 {
		logging.Fatal(fmt.Errorf("no filesystem found"))
	}
	return fsstruct, nil
}

// checkSnapshot checks if there are any snapshots in the subvolume
func checkSnapshot(ctx context.Context, clientsets *k8sutil.Clientsets, operatorNamespace, clusterNamespace, fs, sv, svg string) bool {

	cmd := "ceph"
	args := []string{"fs", "subvolume", "snapshot", "ls", fs, sv, svg, "--format", "json"}

	snapList, err := runCommand(ctx, clientsets, operatorNamespace, clusterNamespace, cmd, args)
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
	cmd := "ceph"
	args := []string{"fs", "subvolumegroup", "ls", fs, "--format", "json"}

	svgList, err := runCommand(ctx, clientsets, operatorNamespace, clusterNamespace, cmd, args)
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
		deleteOmapForSubvolume(ctx, clientsets, OperatorNamespace, CephClusterNamespace, subvol, fs)
		cmd := "ceph"
		args := []string{"fs", "subvolume", "rm", fs, subvol, svg, "--retain-snapshots"}

		_, err := runCommand(ctx, clientsets, OperatorNamespace, CephClusterNamespace, cmd, args)
		if err != nil {
			logging.Fatal(err, "failed to delete subvolume of %s/%s/%s", fs, svg, subvol)
		}
		logging.Info("subvolume %s/%s/%s deleted", fs, svg, subvol)

	} else {
		logging.Info("subvolume %s/%s/%s is not stale", fs, svg, subvol)
	}
}

func getMetadataPoolName(ctx context.Context, clientsets *k8sutil.Clientsets, OperatorNamespace, CephClusterNamespace, fs string) (string, error) {
	fsstruct, err := getFileSystem(ctx, clientsets, OperatorNamespace, CephClusterNamespace)
	if err != nil {
		return "", err
	}

	for _, pool := range fsstruct {
		if pool.Name == fs {
			return pool.MetadataName, nil
		}
	}
	return "", fmt.Errorf("metadataPool not found for %q filesystem", fs)
}

// deleteOmap deletes omap object and key for the given subvolume.
func deleteOmapForSubvolume(ctx context.Context, clientsets *k8sutil.Clientsets, OperatorNamespace, CephClusterNamespace, subVol, fs string) {
	logging.Info("Deleting the omap object and key for subvolume %q", subVol)
	omapkey := getOmapKey(ctx, clientsets, OperatorNamespace, CephClusterNamespace, subVol, fs)
	omapval, subvolId := getOmapVal(subVol)
	poolName, err := getMetadataPoolName(ctx, clientsets, OperatorNamespace, CephClusterNamespace, fs)
	if err != nil || poolName == "" {
		logging.Fatal(fmt.Errorf("pool name not found: %q", err))
	}
	nfsClusterName := getNfsClusterName(ctx, clientsets, OperatorNamespace, CephClusterNamespace, subVol, fs)
	if nfsClusterName != "" {
		exportPath := getNfsExportPath(ctx, clientsets, OperatorNamespace, CephClusterNamespace, nfsClusterName, subvolId)
		if exportPath == "" {
			logging.Info("export path not found for subvol %q: %q", subVol, nfsClusterName)
		} else {
			cmd := "ceph"
			args := []string{"nfs", "export", "delete", nfsClusterName, exportPath}
			_, err := runCommand(ctx, clientsets, OperatorNamespace, CephClusterNamespace, cmd, args)
			if err != nil {
				logging.Fatal(err, "failed to delete export for subvol %q: %q %q", subVol, nfsClusterName, exportPath)
			}
			logging.Info("nfs export: %q %q deleted", nfsClusterName, exportPath)
		}
	}
	if omapval != "" {
		cmd := "rados"
		args := []string{"rm", omapval, "-p", poolName, "--namespace", "csi"}

		// remove omap object.
		_, err := runCommand(ctx, clientsets, OperatorNamespace, CephClusterNamespace, cmd, args)
		if err != nil {
			logging.Fatal(err, "failed to remove omap object for subvolume %q", subVol)
		}
		logging.Info("omap object:%q deleted", omapval)

	}
	if omapkey != "" {
		cmd := "rados"
		args := []string{"rmomapkey", "csi.volumes.default", omapkey, "-p", poolName, "--namespace", "csi"}

		// remove omap key.
		_, err := runCommand(ctx, clientsets, OperatorNamespace, CephClusterNamespace, cmd, args)
		if err != nil {
			logging.Fatal(err, "failed to remove omap key for subvolume %q", subVol)
		}
		logging.Info("omap key:%q deleted", omapkey)

	}
}

// getOmapKey gets the omap key and value details for a given subvolume.
// Deletion of omap object required the subvolumeName which is of format
// csi.volume.subvolume, where subvolume is the name of subvolume that needs to be
// deleted.
// similarly to delete of omap key requires csi.volume.ompakey, where
// omapkey is the pv name which is extracted the omap object.
func getOmapKey(ctx context.Context, clientsets *k8sutil.Clientsets, OperatorNamespace, CephClusterNamespace, subVol, fs string) string {

	poolName, err := getMetadataPoolName(ctx, clientsets, OperatorNamespace, CephClusterNamespace, fs)
	if err != nil || poolName == "" {
		logging.Fatal(fmt.Errorf("pool name not found: %q", err))
	}
	omapval, _ := getOmapVal(subVol)

	args := []string{"getomapval", omapval, "csi.volname", "-p", poolName, "--namespace", "csi", "/dev/stdout"}
	cmd := "rados"
	pvname, err := runCommand(ctx, clientsets, OperatorNamespace, CephClusterNamespace, cmd, args)
	if err != nil || pvname == "" {
		logging.Info("No PV found for subvolume %s: %s", subVol, err)
		return ""
	}
	// omap key is for format csi.volume.pvc-fca205e5-8788-4132-979c-e210c0133182
	// hence, attaching pvname to required prefix.
	omapkey := "csi.volume." + pvname

	return omapkey
}

// getNfsClusterName returns the cluster name from the omap.
// csi.nfs.cluster
// value (26 bytes) :
// 00000000  6f 63 73 2d 73 74 6f 72  61 67 65 63 6c 75 73 74  |my-cluster-cephn|
// 00000010  65 72 2d 63 65 70 68 6e  66 73                    |fs|
// 0000001a
func getNfsClusterName(ctx context.Context, clientsets *k8sutil.Clientsets, OperatorNamespace, CephClusterNamespace, subVol, fs string) string {

	poolName, err := getMetadataPoolName(ctx, clientsets, OperatorNamespace, CephClusterNamespace, fs)
	if err != nil || poolName == "" {
		logging.Fatal(fmt.Errorf("pool name not found %q: %q", poolName, err))
	}
	omapval, _ := getOmapVal(subVol)

	args := []string{"getomapval", omapval, "csi.nfs.cluster", "-p", poolName, "--namespace", "csi", "/dev/stdout"}
	cmd := "rados"
	nfscluster, err := runCommand(ctx, clientsets, OperatorNamespace, CephClusterNamespace, cmd, args)
	if err != nil || nfscluster == "" {
		logging.Info("nfs cluster not found for subvolume %s: %s %s", subVol, poolName, err)
		return ""
	}

	return nfscluster
}

func getNfsExportPath(ctx context.Context, clientsets *k8sutil.Clientsets, OperatorNamespace, CephClusterNamespace, clusterName, subvolId string) string {

	args := []string{"nfs", "export", "ls", clusterName}
	cmd := "ceph"
	exportList, err := runCommand(ctx, clientsets, OperatorNamespace, CephClusterNamespace, cmd, args)
	if err != nil || exportList == "" {
		logging.Info("No export path found for cluster %s: %s", clusterName, err)
		return ""
	}

	var unmarshalexportpath []string
	var exportPath string

	err = json.Unmarshal([]byte(exportList), &unmarshalexportpath)
	if err != nil {
		logging.Info("failed to unmarshal export list: %q", err)
		return ""
	}

	// Extract the value from the unmarshalexportpath
	if len(unmarshalexportpath) > 0 {
		// get the export which matches the subvolume id
		for _, uxp := range unmarshalexportpath {
			if strings.Contains(uxp, subvolId) {
				exportPath = uxp
			}
		}
	}

	return exportPath
}

// func getOmapVal is used to get the omapval from the given subvolume
// omapval is of format csi.volume.427774b4-340b-11ed-8d66-0242ac110005
// which is similar to volume name csi-vol-427774b4-340b-11ed-8d66-0242ac110005
// hence, replacing 'csi-vol-' to 'csi.volume.' to get the omapval
// it also returns the subvolume id
func getOmapVal(subVol string) (string, string) {

	splitSubvol := strings.SplitAfterN(subVol, "-", 3)
	if len(splitSubvol) < 3 {
		return "", ""
	}
	subvolId := splitSubvol[len(splitSubvol)-1]
	omapval := "csi.volume." + subvolId

	return omapval, subvolId
}
