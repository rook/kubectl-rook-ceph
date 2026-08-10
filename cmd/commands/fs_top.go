/*
Copyright 2026 The Rook Authors. All rights reserved.

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

// FsTopCmd represents the fs-top command.
var FsTopCmd = &cobra.Command{
	Use:                "fs-top",
	Short:              "run cephfs-top for real-time CephFS metrics",
	DisableFlagParsing: true,
	Run: func(cmd *cobra.Command, args []string) {
		args = fsTopArgs(args, cephClusterNamespace)
		if err := exec.RunInteractiveCommandInOperatorPod(cmd.Context(), clientSets, "cephfs-top", args, operatorNamespace, cephClusterNamespace); err != nil {
			logging.Fatal(err)
		}
	},
}

func fsTopArgs(args []string, namespace string) []string {
	return append(args, "--id=admin", "--conffile=/var/lib/rook/"+namespace+"/"+namespace+".config")
}
