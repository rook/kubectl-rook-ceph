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
	"github.com/rook/kubectl-rook-ceph/pkg/maintenance"
	"github.com/spf13/cobra"
)

// MaintenanceCmd represents the operator commands
var MaintenanceCmd = &cobra.Command{
	Use:                "maintenance",
	Short:              "Perform maintenance operation on mons and OSDs deployment by scaling it down and creating a maintenance deployment.",
	DisableFlagParsing: true,
	Args:               cobra.ExactArgs(1),
	PreRun: func(cmd *cobra.Command, args []string) {
		verifyOperatorPodIsRunning(cmd.Context(), clientSets)
	},
}

var startMaintenanceCmd = &cobra.Command{
	Use:     "start",
	Short:   "Start a maintenance deployment with an optional alternative ceph container image",
	Args:    cobra.ExactArgs(1),
	Example: "kubectl rook-ceph maintenance start <deployment_name>",
	Run: func(cmd *cobra.Command, args []string) {
		alternateImage := cmd.Flag("alternate-image").Value.String()
		maintenance.StartMaintenance(cmd.Context(), clientSets.Kube, cephClusterNamespace, args[0], alternateImage)
	},
}

var stopMaintenanceCmd = &cobra.Command{
	Use:     "stop",
	Short:   "Stops the maintenance deployment",
	Args:    cobra.ExactArgs(1),
	Example: "kubectl rook-ceph maintenance stop <deployment_name>",
	Run: func(cmd *cobra.Command, args []string) {
		maintenance.StopMaintenance(cmd.Context(), clientSets.Kube, cephClusterNamespace, args[0])
	},
}

func init() {
	MaintenanceCmd.AddCommand(startMaintenanceCmd)
	startMaintenanceCmd.Flags().String("alternate-image", "", "To create deployment with alternate image")
	MaintenanceCmd.AddCommand(stopMaintenanceCmd)
}
