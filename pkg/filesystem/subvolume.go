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

package filesystem

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	osexec "os/exec"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"

	snapclient "github.com/kubernetes-csi/external-snapshotter/client/v8/clientset/versioned/typed/volumesnapshot/v1"
	"github.com/rook/kubectl-rook-ceph/pkg/exec"
	"github.com/rook/kubectl-rook-ceph/pkg/k8sutil"
	"github.com/rook/kubectl-rook-ceph/pkg/logging"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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

type snapshotInfo struct {
	volumeHandle   string
	snapshotHandle string
}

type monitor struct {
	ClusterID string
	Monitors  []string
}

const (
	inUse             = "in-use"
	stale             = "stale"
	staleWithSnapshot = "stale-with-snapshot"
	snapshotRetained  = "snapshot-retained"
)

// CustomExecConfig holds configuration for running commands in a
// user-specified pod instead of the rook operator pod.
type CustomExecConfig struct {
	PodName      string
	PodNamespace string
	Container    string
	MonIP        string
	UserID       string
	UserKey      string
}

// CephFilesystem provides operations on CephFS subvolumes and snapshots.
type CephFilesystem struct {
	Ctx               context.Context
	Clientsets        *k8sutil.Clientsets
	OperatorNamespace string
	ClusterNamespace  string
	RadosNamespace    string
	CustomExecConfig  *CustomExecConfig
}

// List lists CephFS subvolumes. When includeStaleOnly is true,
// filters to subvolumes without matching K8s PVCs.
func (f *CephFilesystem) List(subvolg string, includeStaleOnly bool) {
	subvolumeNames := f.getK8sRefSubvolume()
	snapshotHandles := f.getK8sRefSnapshotHandle()
	f.listCephFSSubvolumes(subvolg, includeStaleOnly, subvolumeNames, snapshotHandles)
}

// checkForExternalStorage checks if the external mode is enabled.
func (f *CephFilesystem) checkForExternalStorage() bool {
	enable := false
	cephclusters, err := f.Clientsets.Rook.CephV1().CephClusters(f.ClusterNamespace).List(f.Ctx, v1.ListOptions{})
	if err != nil {
		logging.Fatal(fmt.Errorf("failed to list CephClusters in namespace %q: %v", f.ClusterNamespace, err))
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
func (f *CephFilesystem) getExternalClusterDetails() (string, string, string) {
	var adminID, adminKey, m string

	scList, err := f.Clientsets.Kube.CoreV1().Secrets(f.ClusterNamespace).List(f.Ctx, v1.ListOptions{})
	if err != nil {
		logging.Fatal(fmt.Errorf("Error fetching secrets in namespace %q: %v", f.ClusterNamespace, err))
	}
	for i := range scList.Items {
		if strings.HasPrefix(scList.Items[i].ObjectMeta.Name, "rook-csi-cephfs-provisioner") {
			data := scList.Items[i].Data
			if data == nil {
				logging.Fatal(fmt.Errorf("Secret data is empty for %s/%s", f.ClusterNamespace, scList.Items[i].ObjectMeta.Name))
			}
			adminID = string(data["adminID"])
			adminKey = string(data["adminKey"])
			break
		}
	}

	cm, err := f.Clientsets.Kube.CoreV1().ConfigMaps(f.ClusterNamespace).Get(f.Ctx, "rook-ceph-mon-endpoints", v1.GetOptions{})
	if err != nil {
		logging.Fatal(fmt.Errorf("Error fetching configmaps %s/rook-ceph-mon-endpoints: %v", f.ClusterNamespace, err))
	}

	if len(cm.Data) == 0 || cm.Data == nil {
		logging.Fatal(fmt.Errorf("Configmap data is empty for %s/rook-ceph-mon-endpoints", f.ClusterNamespace))
	}
	monpoint := cm.Data["csi-cluster-config-json"]
	var monip []monitor
	json.Unmarshal([]byte(monpoint), &monip)
	for _, mp := range monip {
		if len(mp.Monitors) == 0 || mp.Monitors[0] == "" {
			logging.Fatal(fmt.Errorf("mon ip for %s/rook-ceph-mon-endpoints with clusterID:%q is empty", f.ClusterNamespace, mp.ClusterID))
		}
		m = mp.Monitors[0]
	}

	return m, adminID, adminKey
}

// getk8sRefSubvolume returns the k8s ref for the subvolumes
func (f *CephFilesystem) getK8sRefSubvolume() map[string]subVolumeInfo {
	pvList, err := f.Clientsets.ConsumerKube.CoreV1().PersistentVolumes().List(f.Ctx, v1.ListOptions{})
	if err != nil {
		logging.Fatal(fmt.Errorf("Error fetching PVs: %v\n", err))
	}
	subvolumeNames := make(map[string]subVolumeInfo)
	for _, pv := range pvList.Items {
		if pv.Spec.CSI != nil {
			driverName := pv.Spec.CSI.Driver
			if strings.Contains(driverName, "cephfs.csi.ceph.com") {
				volumeHandle := pv.Spec.CSI.VolumeHandle
				prefix := pv.Spec.CSI.VolumeAttributes["volumeNamePrefix"]
				name, err := generateSubvolumeNameFromVolumeHandle(prefix, volumeHandle)
				if err != nil {
					logging.Error(err, "failed to get subvolume name")
					continue
				}
				subvolumeNames[name] = subVolumeInfo{}
			}
		}
	}
	return subvolumeNames
}

// generateSubvolumeNameFromVolumeHandle constructs a subvolume name using the given prefix and volume handle.
// If the prefix is empty, the function extracts the version from the volume handle.
// When the version is `1`, the prefix is set to `csi-vol-`.
// Ref: https://github.com/ceph/ceph-csi/blob/72c09d3d8758d058575d34b2da4b09eb0a591f8f/internal/util/volid.go#L65-L77
// Example:
// volumeHandle: 0001-0011-openshift-storage-0000000000000001-aac40941-9b54-432f-8a63-3b1614a4e024
// prefix: ""
// subvolumeName: csi-vol-aac40941-9b54-432f-8a63-3b1614a4e024
func generateSubvolumeNameFromVolumeHandle(prefix string, volumeHandle string) (string, error) {
	if len(volumeHandle) < 36 {
		return "", fmt.Errorf("volume handle too short to extract subvolume name: %s", volumeHandle)
	}
	if prefix == "" {
		version, err := strconv.ParseInt(volumeHandle[:4], 16, 0)
		if err != nil {
			return "", err
		}
		if version != 1 {
			return "", fmt.Errorf("failed to extract prefix: volume handle %q uses an unsupported version format", volumeHandle)
		}
		prefix = "csi-vol-"
	}
	uuid := volumeHandle[len(volumeHandle)-36:]

	name := fmt.Sprintf("%s%s", prefix, uuid)
	return name, nil
}

// getk8sRefSnapshotHandle returns the snapshothandle for k8s ref of the volume snapshots
func (f *CephFilesystem) getK8sRefSnapshotHandle() map[string]snapshotInfo {
	snapConfig, err := snapclient.NewForConfig(f.Clientsets.ConsumerConfig)
	if err != nil {
		logging.Fatal(err)
	}
	snapList, err := snapConfig.VolumeSnapshotContents().List(f.Ctx, v1.ListOptions{})
	if err != nil {
		// ignore only NotFound
		if apierrors.ReasonForError(err) == v1.StatusReasonNotFound {
			logging.Info("volumesnapshotcontents resource not found, skipping snapshot checks")
			return make(map[string]snapshotInfo)
		}
		logging.Fatal(fmt.Errorf("error fetching volumesnapshotcontents: %v", err))
	}

	snapshotHandles := make(map[string]snapshotInfo)
	for _, snap := range snapList.Items {
		driverName := snap.Spec.Driver
		if snap.Status != nil && snap.Status.SnapshotHandle != nil && strings.Contains(driverName, "cephfs.csi.ceph.com") {
			snapshotHandleId := getSnapshotHandleId(*snap.Status.SnapshotHandle)
			// map the snapshotHandle id to later lookup for the subvol id and
			// match the subvolume snapshot.
			snapshotHandles[snapshotHandleId] = snapshotInfo{}
		}
	}

	return snapshotHandles
}

// getSnapshotHandleId gets the id from snapshothandle
// SnapshotHandle: 0001-0009-rook-ceph-0000000000000001-17b95621-
// 58e8-4676-bc6a-39e928f19d23
// SnapshotHandleId: 17b95621-58e8-4676-bc6a-39e928f19d23
func getSnapshotHandleId(snapshotHandle string) string {
	// get the snaps id from snapshot handle
	splitSnapshotHandle := strings.SplitAfterN(snapshotHandle, "-", 6)
	if len(splitSnapshotHandle) < 6 {
		return ""
	}
	snapshotHandleId := splitSnapshotHandle[len(splitSnapshotHandle)-1]

	return snapshotHandleId
}

// runCommand checks for the presence of externalcluster and runs the command accordingly.
func (f *CephFilesystem) runCommand(cmd string, args []string) (string, error) {
	if f.CustomExecConfig != nil {
		args = append(args,
			"-m", f.CustomExecConfig.MonIP,
			"--id", f.CustomExecConfig.UserID,
			"--key", f.CustomExecConfig.UserKey,
		)
		return exec.RunCommandInPod(
			f.Ctx, f.Clientsets, cmd, args,
			f.CustomExecConfig.PodName,
			f.CustomExecConfig.Container,
			f.CustomExecConfig.PodNamespace, true,
		)
	}

	if f.checkForExternalStorage() {
		m, adminID, adminKey := f.getExternalClusterDetails()
		args = append(args, "-m", m, "--id", adminID, "--key", adminKey)
	}
	list, err := exec.RunCommandInOperatorPod(f.Ctx, f.Clientsets, cmd, args, f.OperatorNamespace, f.ClusterNamespace, true)

	return list, err
}

// listCephFSSubvolumes list all the subvolumes
func (f *CephFilesystem) listCephFSSubvolumes(
	subvolgName string,
	includeStaleOnly bool,
	subvolumeNames map[string]subVolumeInfo,
	snapshotHandles map[string]snapshotInfo,
) {
	fsstruct, err := f.getFileSystem()
	if err != nil {
		logging.Error(err, "failed to get filesystem")
		return
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "Filesystem\tSubvolume\tSubvolumeGroup\tState")

	// collect subvolumes that are not ready (EAGAIN) to show a summary at the end
	var notReadyErrors []string

	// this iterates over the filesystems and subvolumegroup to get the list of subvolumes that exist
	for _, fs := range fsstruct {
		// gets the subvolumegroup in the filesystem
		subvolg, err := f.getSubvolumeGroup(fs.Name)
		if err != nil {
			logging.Error(err, "failed to get subvolume groups")
			continue
		}
		for _, svg := range subvolg {
			if subvolgName != "" && svg.Name != subvolgName {
				continue
			}
			cmd := "ceph"
			args := []string{"fs", "subvolume", "ls", fs.Name, svg.Name, "--format", "json"}
			svList, err := f.runCommand(cmd, args)
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
				state, err := f.getSubvolumeState(fs.Name, sv.Name, svg.Name)
				// subvolume info returns error in case of pending clone or if it is not ready
				// it is suggested to delete the pvc before deleting the subvolume.
				if err != nil {
					if isSubvolumeNotReady(err) {
						// Skip listing this subvolume but remember to report it later
						notReadyErrors = append(notReadyErrors, fmt.Sprintf("%s/%s/%s: %v", fs.Name, svg.Name, sv.Name, err))
						continue
					}
					logging.Fatal(fmt.Errorf("failed to get subvolume state: %q %q", sv.Name, err))

				}
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
					if state == snapshotRetained {
						status = snapshotRetained
						continue
					}
					// check if the stale subvolume has snapshots.
					if f.checkSnapshot(fs.Name, sv.Name, svg.Name, snapshotHandles) {
						status = staleWithSnapshot
					}

				}
				subvolumeNames[sv.Name] = subVolumeInfo{fs.Name, svg.Name, state}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", fs.Name, sv.Name, svg.Name, status)
			}
		}
	}
	w.Flush()

	// After listing, show a concise summary of skipped subvolumes due to not-ready state
	if len(notReadyErrors) > 0 {
		logging.Warning("%d subvolumes were skipped because they are not ready (pending clone or in progress):", len(notReadyErrors))
		for _, errMsg := range notReadyErrors {
			logging.Warning("  %s", errMsg)
		}
		logging.Warning("To avoid stale resources, you may scale down the cephfs deployment before deleting such subvolumes.")
	}
}

// getSubvolumeState returns the state of the subvolume
func (f *CephFilesystem) getSubvolumeState(fsName, SubVol, SubvolumeGroup string) (string, error) {
	cmd := "ceph"
	args := []string{"fs", "subvolume", "info", fsName, SubVol, SubvolumeGroup, "--format", "json"}

	subVolumeInfo, errvol := f.runCommand(cmd, args)
	if errvol != nil {
		// Avoid fmt EXTRA artifacts by formatting the error ourselves
		logging.Error(fmt.Errorf("failed to get subvolume info for %s/%s/%s: %v", fsName, SubvolumeGroup, SubVol, errvol))
		return "", errvol
	}
	var info map[string]any
	err := json.Unmarshal([]byte(subVolumeInfo), &info)
	if err != nil {
		logging.Fatal(fmt.Errorf("failed to unmarshal: %q", err))
	}
	state, ok := info["state"].(string)
	if !ok {
		logging.Fatal(fmt.Errorf("failed to get the state of subvolume: %q", SubVol))
	}
	return state, nil
}

// exitCodeFromError extracts exit code from wrapped errors if possible.
func exitCodeFromError(err error) (int, bool) {
	// errno-style errors
	var errno syscall.Errno
	if errors.As(err, &errno) {
		return int(errno), true
	}
	// errors with ExitStatus()
	type exitStatuser interface{ ExitStatus() int }
	var es exitStatuser
	if errors.As(err, &es) {
		return es.ExitStatus(), true
	}
	// os/exec errors
	var ee *osexec.ExitError
	if errors.As(err, &ee) {
		if status, ok := ee.Sys().(syscall.WaitStatus); ok {
			return status.ExitStatus(), true
		}
	}
	return 0, false
}

// isSubvolumeNotReady detects Ceph EAGAIN "not ready" conditions
func isSubvolumeNotReady(err error) bool {
	if err == nil {
		return false
	}

	// Best check: proper error type match
	if errors.Is(err, syscall.EAGAIN) {
		return true
	}

	// Next best: exit codes 11 or -11 (common Ceph EAGAIN)
	if code, ok := exitCodeFromError(err); ok {
		if code == 11 || code == -11 {
			return true
		}
	}

	// Fallback: minimal string detection
	msg := err.Error()
	if strings.Contains(msg, "EAGAIN") {
		return true
	}
	if strings.Contains(msg, "exit code 11") || strings.Contains(msg, "exit status 11") || strings.Contains(msg, "exit status -11") {
		return true
	}

	return false
}

// getFileSystem gets the list of filesystems in the cluster and returns the details as a struct
func (f *CephFilesystem) getFileSystem() ([]fsStruct, error) {
	cmd := "ceph"
	args := []string{"fs", "ls", "--format", "json"}

	fsList, err := f.runCommand(cmd, args)
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
// it also check for the stale snapshot and if found, deletes the snapshot.
func (f *CephFilesystem) checkSnapshot(fs, sv, svg string, snapshotHandles map[string]snapshotInfo) bool {
	cmd := "ceph"
	args := []string{"fs", "subvolume", "snapshot", "ls", fs, sv, svg, "--format", "json"}

	snapList, err := f.runCommand(cmd, args)
	if err != nil {
		logging.Error(err, "failed to get subvolume snapshots of %q/%q/%q", fs, sv, svg)
		return false
	}
	snap := unMarshaljson(snapList)
	// check for stale subvolume snapshot
	// we have the list of snapshothandleid's from the
	// volumesnapshotcontent. Looking up for snapid in it
	// will confirm if we have stale snapshot or not.
	for _, s := range snap {
		_, snapId := getSnapOmapVal(s.Name)
		// lookup for the snapid in the k8s snapshot handle list
		_, ok := snapshotHandles[snapId]
		if !ok {
			// delete stale snapshot
			f.deleteSnapshot(fs, sv, svg, s.Name)
		}
	}
	if len(snap) == 0 {
		return false
	}
	return true

}

// gets the list of subvolumegroup for the specified filesystem
func (f *CephFilesystem) getSubvolumeGroup(fs string) ([]fsStruct, error) {
	cmd := "ceph"
	args := []string{"fs", "subvolumegroup", "ls", fs, "--format", "json"}

	svgList, err := f.runCommand(cmd, args)
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

// deleteSnapshot deletes the subvolume snapshot
func (f *CephFilesystem) deleteSnapshot(fs, subvol, svg, snap string) {

	f.deleteOmapForSnapshot(snap, fs)
	cmd := "ceph"
	args := []string{"fs", "subvolume", "snapshot", "rm", fs, subvol, snap, svg}

	_, err := f.runCommand(cmd, args)
	if err != nil {
		logging.Fatal(err, "failed to delete subvolume snapshot of %s/%s/%s/%s", fs, svg, subvol, snap)
	}
}

// Delete deletes a stale subvolume after checking it is not referenced by any K8s PV.
func (f *CephFilesystem) Delete(fs, subvol, svg string) {
	k8sSubvolume := f.getK8sRefSubvolume()
	_, check := k8sSubvolume[subvol]
	if !check {
		f.deleteOmapForSubvolume(subvol, fs)
		cmd := "ceph"
		args := []string{"fs", "subvolume", "rm", fs, subvol, svg, "--retain-snapshots"}

		_, err := f.runCommand(cmd, args)
		if err != nil {
			logging.Fatal(err, "failed to delete subvolume of %s/%s/%s", fs, svg, subvol)
		}
		logging.Info("subvolume %s/%s/%s deleted", fs, svg, subvol)

	} else {
		logging.Info("subvolume %s/%s/%s is not stale", fs, svg, subvol)
	}
}

func (f *CephFilesystem) getMetadataPoolName(fs string) (string, error) {
	fsstruct, err := f.getFileSystem()
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

// deleteOmapForSubvolume deletes omap object and key for the given subvolume.
func (f *CephFilesystem) deleteOmapForSubvolume(subVol, fs string) {
	logging.Info("Deleting the omap object and key for subvolume %q", subVol)
	omapkey := f.getOmapKey(subVol, fs)
	omapval, subvolId := getOmapVal(subVol)
	poolName, err := f.getMetadataPoolName(fs)
	if err != nil || poolName == "" {
		logging.Fatal(fmt.Errorf("pool name not found: %q", err))
	}
	nfsClusterName := f.getNfsClusterName(subVol, fs)
	if nfsClusterName != "" {
		exportPath := f.getNfsExportPath(nfsClusterName, subvolId)
		if exportPath == "" {
			logging.Info("export path not found for subvol %q: %q", subVol, nfsClusterName)
		} else {
			cmd := "ceph"
			args := []string{"nfs", "export", "delete", nfsClusterName, exportPath}
			_, err := f.runCommand(cmd, args)
			if err != nil {
				logging.Fatal(err, "failed to delete export for subvol %q: %q %q", subVol, nfsClusterName, exportPath)
			}
			logging.Info("nfs export: %q %q deleted", nfsClusterName, exportPath)
		}
	}
	if omapval != "" {
		cmd := "rados"
		args := []string{"rm", omapval, "-p", poolName, "--namespace", f.RadosNamespace}

		// remove omap object.
		_, err := f.runCommand(cmd, args)
		if err != nil {
			logging.Warning("failed to remove omap object for subvolume %q: %v", subVol, err)
		} else {
			logging.Info("omap object:%q deleted", omapval)
		}
	}
	if omapkey != "" {
		cmd := "rados"
		args := []string{"rmomapkey", "csi.volumes.default", omapkey, "-p", poolName, "--namespace", f.RadosNamespace}

		// remove omap key.
		_, err := f.runCommand(cmd, args)
		if err != nil {
			logging.Warning("failed to remove omap key for subvolume %q: %v", subVol, err)
		} else {
			logging.Info("omap key:%q deleted", omapkey)
		}
	}
}

// deleteOmapForSnapshot deletes omap object and key for the given snapshot.
func (f *CephFilesystem) deleteOmapForSnapshot(snap, fs string) {
	logging.Info("Deleting the omap object and key for snapshot %q", snap)
	snapomapkey := f.getSnapOmapKey(snap, fs)
	snapomapval, _ := getSnapOmapVal(snap)
	poolName, err := f.getMetadataPoolName(fs)
	if err != nil || poolName == "" {
		logging.Fatal(fmt.Errorf("pool name not found: %q", err))
	}
	cmd := "rados"
	if snapomapval != "" {
		args := []string{"rm", snapomapval, "-p", poolName, "--namespace", f.RadosNamespace}

		// remove omap object.
		_, err := f.runCommand(cmd, args)
		if err != nil {
			logging.Warning("failed to remove omap object for snapshot %q: %v", snap, err)
		} else {
			logging.Info("omap object:%q deleted", snapomapval)
		}
	}
	if snapomapkey != "" {
		args := []string{"rmomapkey", "csi.snaps.default", snapomapkey, "-p", poolName, "--namespace", f.RadosNamespace}

		// remove omap key.
		_, err := f.runCommand(cmd, args)
		if err != nil {
			logging.Warning("failed to remove omap key for snapshot %q: %v", snap, err)
		} else {
			logging.Info("omap key:%q deleted", snapomapkey)
		}
	}
}

// getOmapKey gets the omap key and value details for a given subvolume.
// Deletion of omap object required the subvolumeName which is of format
// csi.volume.subvolume, where subvolume is the name of subvolume that needs to be
// deleted.
// similarly to delete of omap key requires csi.volume.ompakey, where
// omapkey is the pv name which is extracted the omap object.
func (f *CephFilesystem) getOmapKey(subVol, fs string) string {
	poolName, err := f.getMetadataPoolName(fs)
	if err != nil || poolName == "" {
		logging.Fatal(fmt.Errorf("pool name not found: %q", err))
	}
	omapval, _ := getOmapVal(subVol)

	args := []string{"getomapval", omapval, "csi.volname", "-p", poolName, "--namespace", f.RadosNamespace, "/dev/stdout"}
	cmd := "rados"
	pvname, err := f.runCommand(cmd, args)
	if err != nil || pvname == "" {
		logging.Info("No PV found for subvolume %s: %s", subVol, err)
		return ""
	}
	// omap key is for format csi.volume.pvc-fca205e5-8788-4132-979c-e210c0133182
	// hence, attaching pvname to required prefix.
	omapkey := "csi.volume." + pvname

	return omapkey
}

// getSnapOmapKey gets the omap key and value details for a given snapshot.
// Deletion of omap object required the snapshotName which is of format
// csi.snap.snapid.
// similarly to delete of omap key requires csi.snap.ompakey, where
// omapkey is the snapshotcontent name which is extracted the omap object.
func (f *CephFilesystem) getSnapOmapKey(snap, fs string) string {
	poolName, err := f.getMetadataPoolName(fs)
	if err != nil || poolName == "" {
		logging.Fatal(fmt.Errorf("pool name not found: %q", err))
	}
	snapomapval, _ := getSnapOmapVal(snap)

	args := []string{"getomapval", snapomapval, "csi.snapname", "-p", poolName, "--namespace", f.RadosNamespace, "/dev/stdout"}
	cmd := "rados"
	snapshotcontentname, err := f.runCommand(cmd, args)
	if snapshotcontentname == "" && err == nil {
		logging.Info("No snapshot content found for snapshot")
		return ""
	}
	if err != nil {
		logging.Fatal(fmt.Errorf("Error getting snapshot content for snapshot %s: %s", snap, err))
	}
	// omap key is for format csi.snap.snapshotcontent-fca205e5-8788-4132-979c-e210c0133182
	// hence, attaching pvname to required prefix.
	snapomapkey := "csi.snap." + snapshotcontentname

	return snapomapkey
}

// getNfsClusterName returns the cluster name from the omap.
// csi.nfs.cluster
// value (26 bytes) :
// 00000000  6f 63 73 2d 73 74 6f 72  61 67 65 63 6c 75 73 74  |my-cluster-cephn|
// 00000010  65 72 2d 63 65 70 68 6e  66 73                    |fs|
// 0000001a
func (f *CephFilesystem) getNfsClusterName(subVol, fs string) string {
	poolName, err := f.getMetadataPoolName(fs)
	if err != nil || poolName == "" {
		logging.Fatal(fmt.Errorf("pool name not found %q: %q", poolName, err))
	}
	omapval, _ := getOmapVal(subVol)

	args := []string{"getomapval", omapval, "csi.nfs.cluster", "-p", poolName, "--namespace", f.RadosNamespace, "/dev/stdout"}
	cmd := "rados"
	nfscluster, err := f.runCommand(cmd, args)
	if err != nil || nfscluster == "" {
		logging.Info("nfs cluster not found for subvolume %s: %s %s", subVol, poolName, err)
		return ""
	}

	return nfscluster
}

func (f *CephFilesystem) getNfsExportPath(clusterName, subvolId string) string {
	args := []string{"nfs", "export", "ls", clusterName}
	cmd := "ceph"
	exportList, err := f.runCommand(cmd, args)
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

// func getSnapOmapVal is used to get the omapval from the given snapshot
// omapval is of format csi.snap.427774b4-340b-11ed-8d66-0242ac110005
// which is similar to volume name csi-snap-427774b4-340b-11ed-8d66-0242ac110005
// hence, replacing 'csi-snap-' to 'csi.snap.'
func getSnapOmapVal(snap string) (string, string) {

	splitSnap := strings.SplitAfterN(snap, "-", 3)
	if len(splitSnap) < 3 {
		return "", ""
	}
	snapId := splitSnap[len(splitSnap)-1]
	snapomapval := "csi.snap." + snapId

	return snapomapval, snapId
}
