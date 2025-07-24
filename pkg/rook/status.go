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
	"context"
	"fmt"
	"os"

	"github.com/rook/kubectl-rook-ceph/pkg/crds"
	"github.com/rook/kubectl-rook-ceph/pkg/k8sutil"
	"github.com/rook/kubectl-rook-ceph/pkg/logging"
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func PrintCustomResourceStatus(ctx context.Context, k8sclientset *k8sutil.Clientsets, clusterNamespace string, arg []string) {
	if len(arg) == 1 && arg[0] == "all" {
		for _, resource := range crds.CephResources {
			printStatus(ctx, k8sclientset, clusterNamespace, resource)
		}
	} else if len(arg) == 1 {
		printStatus(ctx, k8sclientset, clusterNamespace, arg[0])
	} else {
		printStatus(ctx, k8sclientset, clusterNamespace, "cephclusters")
	}

}

func printStatus(ctx context.Context, k8sclientset *k8sutil.Clientsets, clusterNamespace, resource string) {
	items, err := k8sclientset.ListResourcesDynamically(ctx, crds.CephRookIoGroup, crds.CephRookResourcesVersion, resource, clusterNamespace)
	if err != nil {
		logging.Fatal(err)
	}

	if len(items) == 0 {
		logging.Info("resource %s was not found on the cluster", resource)
	}

	for _, crResource := range items {
		status, _, err := unstructured.NestedMap(crResource.Object, "status")
		if err != nil {
			fmt.Printf("Error accessing 'status': %v\n", err)
			os.Exit(1)
		}
		statusYamlData, err := yaml.Marshal(status)
		if err != nil {
			logging.Fatal(err)
		}
		logging.Info("%s %s", resource, crResource.GetName())
		fmt.Println(string(statusYamlData))
	}
}
