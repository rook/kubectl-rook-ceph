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

	"github.com/rook/kubectl-rook-ceph/pkg/crds"
	"github.com/rook/kubectl-rook-ceph/pkg/logging"
	"github.com/rook/kubectl-rook-ceph/pkg/mons"
	"github.com/spf13/cobra"
)

const (
	destroyClusterQuestion = "Are you sure you want to destroy the cluster in namespace %q? If absolutely certain, enter: " + destroyClusterAnswer
	destroyClusterAnswer   = "yes-really-destroy-cluster"
)

// DestroyClusterCmd represents the command for destroy cluster
var DestroyClusterCmd = &cobra.Command{
	Use:   "destroy-cluster",
	Short: "delete ALL data in the Rook cluster and all Rook CRs",

	Run: func(cmd *cobra.Command, args []string) {
		ctx := cmd.Context()
		var answer string
		logging.Warning(destroyClusterQuestion, cephClusterNamespace)
		fmt.Scanf("%s", &answer)
		err := mons.PromptToContinueOrCancel(destroyClusterAnswer, answer)
		if err != nil {
			logging.Fatal(fmt.Errorf("the response %q to confirm the cluster deletion", destroyClusterAnswer))
		}

		logging.Info("proceeding")
		clientsets := getClientsets(ctx)
		crds.DeleteCustomResources(ctx, clientsets, clientsets.Kube, cephClusterNamespace)
	},
}
