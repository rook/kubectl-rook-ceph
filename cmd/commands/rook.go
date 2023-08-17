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
	"fmt"
	"strconv"

	"github.com/rook/kubectl-rook-ceph/pkg/exec"
	"github.com/rook/kubectl-rook-ceph/pkg/logging"
	"github.com/rook/kubectl-rook-ceph/pkg/rook"
	"github.com/spf13/cobra"
)

var json bool

// RookCmd represents the rook commands
var RookCmd = &cobra.Command{
	Use:   "rook",
	Short: "Calls subcommands like `version`, `purge-osd, status` and etc.",
	Args:  cobra.ExactArgs(1),
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Prints rook version",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, _ []string) {
		clientsets := GetClientsets(cmd.Context(), true)
		exec.RunCommandInOperatorPod(cmd.Context(), clientsets, "rook", []string{cmd.Use}, OperatorNamespace, CephClusterNamespace, false, true)
	},
}

var purgeCmd = &cobra.Command{
	Use:   "purge-osd",
	Short: "Permanently remove an OSD from the cluster. Multiple OSDs can be removed with a comma-separated list of IDs.",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		clientsets := GetClientsets(cmd.Context(), true)
		forceflagValue := cmd.Flag("force").Value.String()
		osdID := args[0]
		rook.PurgeOsd(cmd.Context(), clientsets, OperatorNamespace, CephClusterNamespace, osdID, forceflagValue)
	},
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Print the phase and conditions of the CephCluster CR",
	Run: func(cmd *cobra.Command, args []string) {
		json := cmd.Flag("json").Value.String()
		jsonValue, err := strconv.ParseBool(json)
		if err != nil {
			logging.Fatal(fmt.Errorf("failed to parse json flag: %v", err))
		}
		rook.PrintCustomResourceStatus(CephClusterNamespace, args, jsonValue)
	},
}

func init() {
	RookCmd.AddCommand(versionCmd)
	RookCmd.AddCommand(statusCmd)
	RookCmd.AddCommand(purgeCmd)
	statusCmd.PersistentFlags().Bool("json", false, "print status in json format")
	purgeCmd.PersistentFlags().Bool("force", false, "force deletion of an OSD if the OSD still contains data")
}
