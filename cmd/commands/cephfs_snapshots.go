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

package command

import (
	filesystem "github.com/rook/kubectl-rook-ceph/pkg/filesystem"
	"github.com/spf13/cobra"
)

var CephFSSnapshotCmd = &cobra.Command{
	Use:   "snapshot",
	Short: "manages CephFS snapshots",
}

var cephFSSnapshotListCmd = &cobra.Command{
	Use:     "ls",
	Short:   "Print the list of CephFS snapshots.",
	Example: "kubectl rook-ceph snapshot ls",
	PreRun: func(cmd *cobra.Command, args []string) {
		verifyOperatorPodIsRunning(cmd.Context(), clientSets)
	},
	Run: func(cmd *cobra.Command, args []string) {
		ctx := cmd.Context()
		orphanedOnly, _ := cmd.Flags().GetBool("orphaned")
		svg, _ := cmd.Flags().GetString("svg")
		filesystem.SnapshotList(ctx, clientSets, operatorNamespace, cephClusterNamespace, svg, orphanedOnly)
	},
}

var cephFSSnapshotDeleteCmd = &cobra.Command{
	Use:     "delete",
	Short:   "Deletes a CephFS snapshot.",
	Args:    cobra.ExactArgs(3),
	Example: "kubectl rook-ceph snapshot delete <filesystem> <subvol> <snapshot> ",
	PreRun: func(cmd *cobra.Command, args []string) {
		verifyOperatorPodIsRunning(cmd.Context(), clientSets)
	},
	Run: func(cmd *cobra.Command, args []string) {
		ctx := cmd.Context()
		fs := args[0]
		subvol := args[1]
		snap := args[2]
		svg, _ := cmd.Flags().GetString("svg")
		filesystem.SnapshotDelete(ctx, clientSets, operatorNamespace, cephClusterNamespace, fs, subvol, snap, svg)
	},
}

func init() {
	CephFSSnapshotCmd.AddCommand(cephFSSnapshotListCmd)
	cephFSSnapshotListCmd.PersistentFlags().Bool("orphaned", false, "List only orphaned snapshots")
	CephFSSnapshotCmd.PersistentFlags().String("svg", "csi", "The name of the subvolume group")
	CephFSSnapshotCmd.AddCommand(cephFSSnapshotDeleteCmd)

}
