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
	Args:  cobra.ExactArgs(1),
}

var listCmd = &cobra.Command{
	Use:   "ls",
	Short: "Print the list of subvolumes.",
	Run: func(cmd *cobra.Command, args []string) {
		ctx := cmd.Context()
		clientsets := GetClientsets(ctx)
		VerifyOperatorPodIsRunning(ctx, clientsets, OperatorNamespace, CephClusterNamespace)
		staleSubvol, _ := cmd.Flags().GetBool("stale")
		subvolume.List(ctx, clientsets, OperatorNamespace, CephClusterNamespace, staleSubvol)
	},
}

var deleteCmd = &cobra.Command{
	Use:                "delete",
	Short:              "Deletes a stale subvolume.",
	DisableFlagParsing: true,
	Args:               cobra.ExactArgs(3),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := cmd.Context()
		clientsets := GetClientsets(ctx)
		VerifyOperatorPodIsRunning(ctx, clientsets, OperatorNamespace, CephClusterNamespace)
		subList := args[0]
		fs := args[1]
		svg := args[2]
		subvolume.Delete(ctx, clientsets, OperatorNamespace, CephClusterNamespace, subList, fs, svg)
	},
}

func init() {
	SubvolumeCmd.AddCommand(listCmd)
	SubvolumeCmd.PersistentFlags().Bool("stale", false, "List only stale subvolumes")
	SubvolumeCmd.AddCommand(deleteCmd)
}
