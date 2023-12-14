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
	"github.com/rook/kubectl-rook-ceph/pkg/debug"
	"github.com/spf13/cobra"
)

// OperatorCmd represents the operator commands
var DebugCmd = &cobra.Command{
	Use:                "debug",
	Short:              "Debug a deployment by scaling it down and creating a debug copy. This is supported for mons and OSDs only",
	DisableFlagParsing: true,
	Args:               cobra.ExactArgs(1),
	PreRun: func(cmd *cobra.Command, args []string) {
		verifyOperatorPodIsRunning(cmd.Context(), clientSets)
	},
}

var startDebugCmd = &cobra.Command{
	Use:   "start",
	Short: "Start debugging a deployment with an optional alternative ceph container image",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		alternateImage := cmd.Flag("alternate-image").Value.String()
		debug.StartDebug(cmd.Context(), clientSets.Kube, cephClusterNamespace, args[0], alternateImage)
	},
}

var stopDebugCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop debugging a deployment",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		debug.StopDebug(cmd.Context(), clientSets.Kube, cephClusterNamespace, args[0])
	},
}

func init() {
	DebugCmd.AddCommand(startDebugCmd)
	startDebugCmd.Flags().String("alternate-image", "", "To create deployment with alternate image")
	DebugCmd.AddCommand(stopDebugCmd)
}
