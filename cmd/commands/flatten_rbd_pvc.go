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
	flatten_rbd_pvc "github.com/rook/kubectl-rook-ceph/pkg/flatten-rbd-pvc"

	"github.com/spf13/cobra"
)

var namespace string
var allowInUse bool

// FlattenRBDPVCCmd represents the rook commands
var FlattenRBDPVCCmd = &cobra.Command{
	Use:   "flatten-rbd-pvc",
	Short: "Flatten the RBD image corresponding to the target RBD PVC",
	Long: `Flatten the RBD image corresponding to the target RBD PVC.
The target RBD PVC must be a cloned image and must be created by ceph-csi.
This command removes the corresponding temporary cloned image[1]
if the target PVC was cloned from another PVC.

[1]: https://github.com/ceph/ceph-csi/blob/devel/docs/design/proposals/rbd-snap-clone.md`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		flatten_rbd_pvc.FlattenRBDPVC(cmd.Context(), clientSets, operatorNamespace, cephClusterNamespace, namespace, args[0], allowInUse)
	},
}

func init() {
	FlattenRBDPVCCmd.Flags().StringVarP(&namespace, "namespace", "n", "default", "pvc's namespace")
	FlattenRBDPVCCmd.Flags().BoolVarP(&allowInUse, "allow-in-use", "a", false, "allow to flatten in-use image")
}
