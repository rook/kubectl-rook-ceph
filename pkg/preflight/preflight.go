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

// Package preflight checks whether a Kubernetes cluster's nodes can run a Rook Ceph cluster,
// before the cluster is created.
package preflight

import (
	"context"
	"fmt"

	"github.com/rook/kubectl-rook-ceph/pkg/health"
	"github.com/rook/kubectl-rook-ceph/pkg/logging"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// DefaultImage is the Ceph image used to inspect the nodes. It matches the image in the Rook
// example manifests.
const DefaultImage = "quay.io/ceph/ceph:v20.2.4"

// Options configures a preflight run.
type Options struct {
	// Image is the Ceph image used to inspect the nodes.
	Image string
	// Namespace is where the node inspection pods run.
	Namespace string
	// NodeSelector limits the nodes that are checked.
	NodeSelector string
	// SkipNodeInspection skips the privileged pods, and with them the disk, rbd and memory checks.
	SkipNodeInspection bool
	OutputFormat       string
	Verbose            bool
}

// Run checks the cluster and prints a report. It returns true if any check is critical or failed.
func Run(ctx context.Context, k8sclientset kubernetes.Interface, opts Options) bool {
	image := opts.Image
	if image == "" {
		image = DefaultImage
	}

	var results []health.CheckResult

	logging.Plain("Checking %s...", CheckKubernetesVersion)
	results = append(results, checkKubernetesVersion(k8sclientset.Discovery().ServerVersion()))

	nodeList, err := k8sclientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{LabelSelector: opts.NodeSelector})
	if err != nil {
		logging.Fatal(fmt.Errorf("failed to list nodes: %v", err))
	}
	nodes := nodeList.Items

	logging.Plain("Checking %s...", CheckNodeArchitecture)
	results = append(results, checkNodeArchitecture(nodes))
	logging.Plain("Checking %s...", CheckMonPlacement)
	results = append(results, checkMonPlacement(nodes))

	var probes []nodeProbe
	if !opts.SkipNodeInspection {
		var names []string
		for _, node := range schedulableNodes(nodes) {
			names = append(names, node.Name)
		}
		logging.Plain("Inspecting %d node(s) with image %s in namespace %s...", len(names), image, opts.Namespace)
		probes = probeNodes(ctx, k8sclientset, opts.Namespace, image, names)
	}

	logging.Plain("Checking %s...", CheckKernel)
	results = append(results, checkKernel(nodes, probes))
	if !opts.SkipNodeInspection {
		logging.Plain("Checking %s...", CheckDisks)
		results = append(results, checkDisks(probes))
		logging.Plain("Checking %s...", CheckMemory)
		results = append(results, checkMemory(nodes, probes))
	}

	health.FormatReport("PREFLIGHT REPORT", opts.Namespace, results, opts.OutputFormat, opts.Verbose)

	for _, r := range results {
		if r.Status == health.StatusCritical || r.Status == health.StatusError {
			return true
		}
	}
	return false
}
