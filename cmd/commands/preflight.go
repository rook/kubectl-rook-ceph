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
	"os"

	"github.com/rook/kubectl-rook-ceph/pkg/preflight"
	"github.com/spf13/cobra"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	preflightImage          string
	preflightProbeNamespace string
	preflightNodeSelector   string
	preflightSkipInspection bool
	preflightOutput         string
	preflightVerbose        bool
)

var PreflightCmd = &cobra.Command{
	Use:   "preflight",
	Short: "check whether the nodes can run a Rook Ceph cluster, before creating it",
	Long: `Checks the Kubernetes version, node architecture, mon placement, kernel, disks and memory.

To inspect disks and the rbd module, a short-lived privileged pod runs on each schedulable node,
using the Ceph image. Use --skip-node-inspection to run only the checks that need no pods.

The command exits with status 1 if any check is critical.`,
	Args: cobra.NoArgs,
	// Preflight runs before Rook is installed, so it does not require the operator and cluster
	// namespaces to exist.
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		setupClients(cmd.Context())
	},
	Run: func(cmd *cobra.Command, _ []string) {
		ctx := cmd.Context()
		namespace := preflightProbeNamespace
		if namespace == "" {
			namespace = "default"
			if _, err := clientSets.Kube.CoreV1().Namespaces().Get(ctx, cephClusterNamespace, v1.GetOptions{}); err == nil {
				namespace = cephClusterNamespace
			}
		}

		failed := preflight.Run(ctx, clientSets.Kube, preflight.Options{
			Image:              preflightImage,
			Namespace:          namespace,
			NodeSelector:       preflightNodeSelector,
			SkipNodeInspection: preflightSkipInspection,
			OutputFormat:       preflightOutput,
			Verbose:            preflightVerbose,
		})
		if failed {
			os.Exit(1)
		}
	},
}

func init() {
	PreflightCmd.Flags().StringVar(&preflightImage, "image", preflight.DefaultImage, "Ceph image for the node inspection pods")
	PreflightCmd.Flags().StringVar(&preflightProbeNamespace, "probe-namespace", "", "namespace for the node inspection pods (default: the cluster namespace if it exists, otherwise default)")
	PreflightCmd.Flags().StringVar(&preflightNodeSelector, "node-selector", "", "label selector for the nodes to check")
	PreflightCmd.Flags().BoolVar(&preflightSkipInspection, "skip-node-inspection", false, "skip the privileged pods, and the disk, rbd and memory checks")
	PreflightCmd.Flags().StringVarP(&preflightOutput, "output", "o", "text", "output format: text, json, yaml")
	PreflightCmd.Flags().BoolVar(&preflightVerbose, "verbose", false, "show every node and device")
}
