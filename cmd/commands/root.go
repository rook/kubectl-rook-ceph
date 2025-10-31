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
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/rook/kubectl-rook-ceph/pkg/exec"
	"github.com/rook/kubectl-rook-ceph/pkg/k8sutil"
	"github.com/rook/kubectl-rook-ceph/pkg/logging"
	rookclient "github.com/rook/rook/pkg/client/clientset/versioned"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	k8s "k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

var (
	operatorNamespace    string
	cephClusterNamespace string
	clientSets           *k8sutil.Clientsets
	clientConfig         clientcmd.ClientConfig
)

// rookCmd represents the rook command
var RootCmd = &cobra.Command{
	Use:              "rook-ceph",
	Short:            "kubectl rook-ceph provides common management and troubleshooting tools for Ceph.",
	Args:             cobra.MinimumNArgs(1),
	TraverseChildren: true,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Get the effective namespace from client config
		effectiveNamespace, _, err := clientConfig.Namespace()
		if err != nil {
			logging.Fatal(err)
		}
		cephClusterNamespace = effectiveNamespace

		if cephClusterNamespace != "" && operatorNamespace == "" {
			operatorNamespace = cephClusterNamespace
		}
		clientSets = getClientsets(cmd.Context())
		preValidationCheck(cmd.Context(), clientSets)
	},
}

func init() {
	// Initialize client configuration with all standard kubectl flags first
	clientConfig = defaultClientConfig(RootCmd.PersistentFlags())
	RootCmd.PersistentFlags().StringVar(&operatorNamespace, "operator-namespace", "", "Kubernetes namespace where rook operator is running")

	// Set default ceph cluster namespace
	cephClusterNamespace = "rook-ceph"
}

// defaultClientConfig creates a client configuration with all standard kubectl flags
func defaultClientConfig(flags *pflag.FlagSet) clientcmd.ClientConfig {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()

	// Create config overrides and bind standard kubectl flags
	overrides := &clientcmd.ConfigOverrides{}
	clientcmd.BindOverrideFlags(overrides, flags, clientcmd.RecommendedConfigOverrideFlags(""))

	// Bind kubeconfig-related flags manually
	flags.StringVar(&loadingRules.ExplicitPath, "kubeconfig", "", "Path to the kubeconfig file to use for CLI requests")

	// Customize the namespace flag description to indicate our default
	if namespaceFlag := flags.Lookup("namespace"); namespaceFlag != nil {
		namespaceFlag.Usage = "If present, the namespace scope for this CLI request (defaults to 'rook-ceph')"
	}

	// Create the client config
	baseConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		loadingRules,
		overrides,
	)

	// Return a wrapper that can override namespace when needed
	return &namespacedClientConfig{
		base: baseConfig,
	}
}

// namespacedClientConfig wraps a client config and can override the namespace
type namespacedClientConfig struct {
	base clientcmd.ClientConfig
}

func (c *namespacedClientConfig) RawConfig() (clientcmdapi.Config, error) {
	return c.base.RawConfig()
}

func (c *namespacedClientConfig) ClientConfig() (*rest.Config, error) {
	return c.base.ClientConfig()
}

func (c *namespacedClientConfig) Namespace() (string, bool, error) {
	// Check if namespace was explicitly set via flag
	if baseNs, overridden, err := c.base.Namespace(); err == nil && overridden {
		// Namespace was explicitly set via flags, use it
		return baseNs, true, nil
	}
	// Use our default namespace (rook-ceph)
	return cephClusterNamespace, false, nil
}

func (c *namespacedClientConfig) ConfigAccess() clientcmd.ConfigAccess {
	return c.base.ConfigAccess()
}

func getClientsets(ctx context.Context) *k8sutil.Clientsets {
	var err error

	clientsets := &k8sutil.Clientsets{}

	// Use the client configuration that includes all standard kubectl flags
	clientsets.KubeConfig, err = clientConfig.ClientConfig()
	if err != nil {
		logging.Fatal(err)
	}

	clientsets.Rook, err = rookclient.NewForConfig(clientsets.KubeConfig)
	if err != nil {
		logging.Fatal(err)
	}

	clientsets.Kube, err = k8s.NewForConfig(clientsets.KubeConfig)
	if err != nil {
		logging.Fatal(err)
	}

	clientsets.Dynamic, err = dynamic.NewForConfig(clientsets.KubeConfig)
	if err != nil {
		logging.Fatal(err)
	}

	return clientsets
}

func preValidationCheck(ctx context.Context, k8sclientset *k8sutil.Clientsets) {
	_, err := k8sclientset.Kube.CoreV1().Namespaces().Get(ctx, operatorNamespace, v1.GetOptions{})
	if err != nil {
		logging.Fatal(fmt.Errorf("operator namespace '%s' does not exist. %v", operatorNamespace, err))
	}
	_, err = k8sclientset.Kube.CoreV1().Namespaces().Get(ctx, cephClusterNamespace, v1.GetOptions{})
	if err != nil {
		logging.Fatal(fmt.Errorf("cephCluster namespace '%s' does not exist. %v", cephClusterNamespace, err))
	}
}

func verifyOperatorPodIsRunning(ctx context.Context, k8sclientset *k8sutil.Clientsets) {
	rookVersionOutput, err := exec.RunCommandInOperatorPod(ctx, k8sclientset, "rook", []string{"version"}, operatorNamespace, cephClusterNamespace, true)
	if err != nil {
		logging.Fatal(err, "failed to get rook version")
	}
	rookVersion := trimGoVersionFromRookVersion(rookVersionOutput)
	if strings.Contains(rookVersion, "alpha") || strings.Contains(rookVersion, "beta") {
		logging.Warning("rook version '%s' is running a pre-release version of Rook.", rookVersion)
		fmt.Println()
	}
}

func trimGoVersionFromRookVersion(rookVersion string) string {
	re := regexp.MustCompile("(?m)[\r\n]+^.*go: go.*$") // remove the go version from the output
	rookVersion = re.ReplaceAllString(rookVersion, "")
	rookVersion = strings.TrimSpace(rookVersion) // remove any trailing newlines

	return rookVersion
}
