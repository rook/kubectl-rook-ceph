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
)

var scriptPrintSpecificCRStatus = `
kubectl -n %s get %s -ojson | jq --monochrome-output '.items[].status'
`

var getCrdList = `
kubectl -n  %s get crd | awk '{print $1}' | sed '1d'
`

func PrintCustomResourceStatus(clusterNamespace string, arg []string) {
	if len(arg) == 1 && arg[0] == "all" {
		command := fmt.Sprintf(getCrdList, clusterNamespace)
		allCRs := strings.Split(exec.ExecuteBashCommand(command), "\n")
		fmt.Println(allCRs[0])
		for _, cr := range allCRs {
			fmt.Println(cr)
			command := fmt.Sprintf(scriptPrintSpecificCRStatus, clusterNamespace, cr)
			fmt.Println(exec.ExecuteBashCommand(command))
		}

	} else if len(arg) == 1 {
		command := fmt.Sprintf(scriptPrintSpecificCRStatus, clusterNamespace, arg[0])
		fmt.Println(exec.ExecuteBashCommand(command))
	} else {
		command := fmt.Sprintf(scriptPrintSpecificCRStatus, clusterNamespace, "cephclusters.ceph.rook.io")
		fmt.Println(exec.ExecuteBashCommand(command))
	}
}
