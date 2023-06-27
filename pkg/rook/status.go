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

package rook

import (
	"fmt"
	"strings"

	"github.com/rook/kubectl-rook-ceph/pkg/exec"
	"github.com/rook/kubectl-rook-ceph/pkg/logging"
)

var scriptPrintSpecificCRStatus = `
kubectl -n %s get %s
`

var getCrdList = `
kubectl -n  %s get crd | awk '{print $1}' | sed '1d'
`

func PrintCustomResourceStatus(clusterNamespace string, arg []string, json bool) {
	if len(arg) == 1 && arg[0] == "all" {
		command := fmt.Sprintf(getCrdList, clusterNamespace)
		allCRs := strings.Split(exec.ExecuteBashCommand(command), "\n")
		allCRs = allCRs[:len(allCRs)-1] // remove last empty line which is not a CR
		for _, cr := range allCRs {
			logging.Info(cr)
			if json {
				cr = concatenateJsonflag(cr)
			}
			command := fmt.Sprintf(scriptPrintSpecificCRStatus, clusterNamespace, cr)
			fmt.Println(exec.ExecuteBashCommand(command))
		}

	} else if len(arg) == 1 {
		if json {
			arg[0] = concatenateJsonflag(arg[0])
		}

		command := fmt.Sprintf(scriptPrintSpecificCRStatus, clusterNamespace, arg[0])
		logging.Info(exec.ExecuteBashCommand(command))
	} else {
		arg := "cephclusters.ceph.rook.io"
		if json {
			arg = concatenateJsonflag(arg)
		}

		command := fmt.Sprintf(scriptPrintSpecificCRStatus, clusterNamespace, arg)
		logging.Info(exec.ExecuteBashCommand(command))
	}
}

func concatenateJsonflag(arg string) string {
	return arg + " -o json"
}
