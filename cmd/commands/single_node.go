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

	"github.com/rook/kubectl-rook-ceph/pkg/create"
	"github.com/rook/kubectl-rook-ceph/pkg/logging"
	"github.com/spf13/cobra"
)

var storageClassName string

// CreateCmd represents the create command
var CreateCmd = &cobra.Command{
	Use:                "create",
	Short:              "Create is the parent command for the sample-cluster",
	DisableFlagParsing: true,
	Args:               cobra.ExactArgs(1),
}

var sampleCmd = &cobra.Command{
	Use:   "sample-cluster",
	Short: "Create single node rook-ceph cluster",
	Long: `
	"kubectl rook-ceph create sample-cluster creates single node rook-ceph cluster based on the rook/ceph repo default examples configuration.

	For pvc based cluster pass the flag '--storageclass=<name of the storageclass>'

	Also,if you are creating this in minikube make sure to some minikube driver."
	`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		clientsets := GetClientsets(cmd.Context(), false)
		if storageClassName == "" {
			logging.Fatal(fmt.Errorf("please pass the flag --storageclass=<name of the storageclass>"))
		}
		create.SampleCluster(cmd.Context(), clientsets, OperatorNamespace, CephClusterNamespace, storageClassName)
	},
}

func init() {
	CreateCmd.AddCommand(sampleCmd)
	sampleCmd.Flags().StringVar(&storageClassName, "storageclass", "", "Name of the storageclass to be used for the cluster")
}
