/*
Copyright 2024 The Rook Authors. All rights reserved.

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

package command

import (
	"encoding/json"
	"fmt"

	"github.com/rook/kubectl-rook-ceph/pkg/exec"
	"github.com/rook/kubectl-rook-ceph/pkg/logging"

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type TrashLsOutput struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

var namespace string
var allowInUse bool

// FlattenRBDPVCCmd represents the rook commands
var FlattenRBDPVCCmd = &cobra.Command{
	Use:   "flatten-rbd-pvc",
	Short: "Flatten the RBD image corresponding to the target RBD PVC",
	Long: `Flatten the RBD image corresponding to the target RBD PVC.
The target RBD PVC must be a cloned image and must be created by ceph-csi.
In other words, you can't flatten static PVC by this command.
This command removes the corresponding temporary cloned image[1]
if the target PVC was cloned from another PVC.

[1]: https://github.com/ceph/ceph-csi/blob/328e4e5a0fa06521c5508b94a5b7ff250ada1681/docs/design/proposals/rbd-snap-clone.md.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		pvcName := args[0]
		pvc, err := clientSets.Kube.CoreV1().PersistentVolumeClaims(namespace).Get(cmd.Context(), pvcName, metav1.GetOptions{})
		if err != nil {
			logging.Fatal(err, "failed to get pvc %s/%s", namespace, pvcName)
		}
		if pvc.DeletionTimestamp != nil {
			logging.Fatal(fmt.Errorf("pvc %s is deleting", pvcName))
		}
		if pvc.Status.Phase != corev1.ClaimBound {
			logging.Fatal(fmt.Errorf("pvc %s is not bound", pvcName))
		}
		pvName := pvc.Spec.VolumeName
		pv, err := clientSets.Kube.CoreV1().PersistentVolumes().Get(cmd.Context(), pvName, metav1.GetOptions{})
		if err != nil {
			logging.Fatal(fmt.Errorf("failed to get pv %s", pvName))
		}
		imageName, ok := pv.Spec.CSI.VolumeAttributes["imageName"]
		if !ok {
			logging.Fatal(fmt.Errorf("pv %s would not be a CSI PVC", pvName))
		}
		poolName, ok := pv.Spec.CSI.VolumeAttributes["pool"]
		logging.Fatal(fmt.Errorf("pv %s would not be a CSI PVC", pvName))

		// TODO: consider clone PVC from snapshot. see https://github.com/rook/kubectl-rook-ceph/pull/229#discussion_r1467429104
		tempImageName := imageName + "-temp"
		if !allowInUse {
			// TODO: check whether the target PVC is mounted
			logging.Fatal(fmt.Errorf("can't flatten in-use pvc %s", pvcName))
		}
		logging.Info("removing the temporary RBD image %s/%s if exist", poolName, tempImageName)
		// TODO: cosider radosnamespace. see https://github.com/rook/kubectl-rook-ceph/pull/229#discussion_r1467429104
		_, err = exec.RunCommandInOperatorPod(cmd.Context(), clientSets, "rbd", []string{"-p", poolName, "trash", "mv", tempImageName}, cephClusterNamespace, operatorNamespace, false)
		if err != nil {
			logging.Fatal(fmt.Errorf("failed to move rbd image %s/%s to trash", poolName, tempImageName))
		}
		// TODO: avoid heavy operation (trash ls). see https://github.com/rook/kubectl-rook-ceph/pull/229#discussion_r1467433441
		out, err := exec.RunCommandInOperatorPod(cmd.Context(), clientSets, "rbd", []string{"-p", poolName, "trash", "ls", "--format=json"}, cephClusterNamespace, operatorNamespace, true)
		if err != nil {
			logging.Fatal(fmt.Errorf(""))
		}
		var data []TrashLsOutput
		json.Unmarshal([]byte(out), &data)
		var id string
		for _, d := range data {
			if d.Name == tempImageName {
				id = d.ID
				break
			}
		}
		if id != "" {
			_, err = exec.RunCommandInOperatorPod(cmd.Context(), clientSets, "ceph", []string{"rbd", "task", "add", "trash", "remove", fmt.Sprintf("%s/%s", poolName, id)}, cephClusterNamespace, operatorNamespace, false)
			if err != nil {
				logging.Fatal(fmt.Errorf("failed to create a task to remove %s/%s from trash", poolName, id))
			}
		}
		logging.Info("flattening the target RBD image %s/%s", poolName, imageName)
		_, err = exec.RunCommandInOperatorPod(cmd.Context(), clientSets, "rbd", []string{"-p", poolName, "flatten", imageName}, cephClusterNamespace, operatorNamespace, false)
		if err != nil {
			logging.Fatal(fmt.Errorf("failed to flatten %s/%s", poolName, imageName))
		}
	},
}

func init() {
	FlattenRBDPVCCmd.Flags().StringVarP(&namespace, "namespace", "n", "default", "pvc's namespace")
	FlattenRBDPVCCmd.Flags().BoolVarP(&allowInUse, "allow-in-use", "a", false, "allow to flatten in-use image")
}
