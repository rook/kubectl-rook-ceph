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
	rbd "github.com/rook/kubectl-rook-ceph/pkg/rbd"

	"github.com/rook/kubectl-rook-ceph/pkg/exec"
	"github.com/rook/kubectl-rook-ceph/pkg/logging"
	"github.com/spf13/cobra"
)

// RbdCmd represents the rbd command
var RbdCmd = &cobra.Command{
	Use:                "rbd",
	Short:              "call a 'rbd' CLI command with arbitrary args",
	DisableFlagParsing: true,
	Args:               cobra.MinimumNArgs(1),
	PreRun: func(cmd *cobra.Command, args []string) {
		verifyOperatorPodIsRunning(cmd.Context(), clientSets)
	},
	Run: func(cmd *cobra.Command, args []string) {
		_, err := exec.RunCommandInOperatorPod(cmd.Context(), clientSets, cmd.Use, args, operatorNamespace, cephClusterNamespace, false)
		if err != nil {
			logging.Fatal(err)
		}
	},
}

var listCmdRbd = &cobra.Command{
	Use:   "ls",
	Short: "Print the list of rbd images.",
	Run: func(cmd *cobra.Command, args []string) {
		ctx := cmd.Context()
		rbd.ListImages(ctx, clientSets, operatorNamespace, cephClusterNamespace)
	},
}

func init() {
	RbdCmd.AddCommand(listCmdRbd)
}
