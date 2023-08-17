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
	"github.com/rook/kubectl-rook-ceph/pkg/exec"
	"github.com/rook/kubectl-rook-ceph/pkg/logging"
	"github.com/spf13/cobra"
)

// CephCmd represents the ceph command
var CephCmd = &cobra.Command{
	Use:                "ceph",
	Short:              "call a 'ceph' CLI command with arbitrary args",
	DisableFlagParsing: true,
	Args:               cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		clientsets := GetClientsets(cmd.Context(), true)
		logging.Info("running 'ceph' command with args: %v", args)
		exec.RunCommandInOperatorPod(cmd.Context(), clientsets, cmd.Use, args, OperatorNamespace, CephClusterNamespace, false, true)
	},
}
