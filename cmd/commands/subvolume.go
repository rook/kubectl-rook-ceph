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

package command

import (
	subvolume "github.com/rook/kubectl-rook-ceph/pkg/filesystem"
	"github.com/spf13/cobra"
)

var SubvolumeCmd = &cobra.Command{
	Use:   "subvolume",
	Short: "manages stale subvolumes",
	PreRun: func(cmd *cobra.Command, args []string) {
		verifyOperatorPodIsRunning(cmd.Context(), clientSets)
	},
	Args: cobra.ExactArgs(1),
}

var listCmd = &cobra.Command{
	Use:   "ls",
	Short: "Print the list of subvolumes.",
	Run: func(cmd *cobra.Command, args []string) {
		ctx := cmd.Context()
		staleSubvol, _ := cmd.Flags().GetBool("stale")
		subvolume.List(ctx, clientSets, operatorNamespace, cephClusterNamespace, staleSubvol)
	},
}

var deleteCmd = &cobra.Command{
	Use:     "delete",
	Short:   "Deletes a stale subvolume.",
	Args:    cobra.RangeArgs(2, 3),
	Hidden:  true,
	Example: "kubectl rook-ceph delete <filesystem> <subvolume> [subvolumegroup]",
	Run: func(cmd *cobra.Command, args []string) {
		ctx := cmd.Context()
		fs := args[0]
		subvol := args[1]
		svg := "csi"
		if len(args) > 2 {
			svg = args[2]
		}
		subvolume.Delete(ctx, clientSets, operatorNamespace, cephClusterNamespace, fs, subvol, svg)
	},
}

func init() {
	SubvolumeCmd.AddCommand(listCmd)
	SubvolumeCmd.PersistentFlags().Bool("stale", false, "List only stale subvolumes")
	SubvolumeCmd.AddCommand(deleteCmd)
}
