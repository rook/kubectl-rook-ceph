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

	"github.com/rook/kubectl-rook-ceph/pkg/mons"
	"github.com/spf13/cobra"
)

// MonCmd represents the mons command
var MonCmd = &cobra.Command{
	Use:                "mons",
	Short:              "Output mon endpoints",
	DisableFlagParsing: true,
	Args:               cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			fmt.Println(mons.GetMonEndpoint(cmd.Context(), clientSets.Kube, cephClusterNamespace))
		}
	},
}

// RestoreQuorum represents the mons command
var RestoreQuorum = &cobra.Command{
	Use:                "restore-quorum",
	Short:              "When quorum is lost, restore quorum to the remaining healthy mon",
	DisableFlagParsing: true,
	Args:               cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		mons.RestoreQuorum(cmd.Context(), clientSets, operatorNamespace, cephClusterNamespace, args[0])
	},
}

func init() {
	MonCmd.AddCommand(RestoreQuorum)
}
