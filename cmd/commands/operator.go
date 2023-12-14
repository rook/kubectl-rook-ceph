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
	k8sutil "github.com/rook/kubectl-rook-ceph/pkg/k8sutil"
	"github.com/spf13/cobra"
)

// OperatorCmd represents the operator commands
var OperatorCmd = &cobra.Command{
	Use:                "operator",
	Short:              "Calls subcommands like `restart`  and `set <key> <value>` to update  rook-ceph-operator-config configmap",
	DisableFlagParsing: true,
	Args:               cobra.ExactArgs(1),
}

var restartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart rook-ceph-operator pod",
	Args:  cobra.NoArgs,
	PreRun: func(cmd *cobra.Command, args []string) {
		verifyOperatorPodIsRunning(cmd.Context(), clientSets)
	},
	Run: func(cmd *cobra.Command, _ []string) {
		k8sutil.RestartDeployment(cmd.Context(), clientSets.Kube, operatorNamespace, "rook-ceph-operator")
	},
}

var setCmd = &cobra.Command{
	Use:   "set",
	Short: "Set the property in the rook-ceph-operator-config configmap.",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		k8sutil.UpdateConfigMap(cmd.Context(), clientSets.Kube, operatorNamespace, "rook-ceph-operator-config", args[0], args[1])
	},
}

func init() {
	OperatorCmd.AddCommand(restartCmd)
	OperatorCmd.AddCommand(setCmd)
}
