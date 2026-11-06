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
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func Test_completionHonorsKubeconfigParsedAfterPersistentPreRun(t *testing.T) {
	writeKubeconfig := func(path, server string) {
		config := clientcmdapi.NewConfig()
		config.Clusters["cluster"] = &clientcmdapi.Cluster{Server: server}
		config.AuthInfos["user"] = &clientcmdapi.AuthInfo{}
		config.Contexts["context"] = &clientcmdapi.Context{Cluster: "cluster", AuthInfo: "user"}
		config.CurrentContext = "context"
		require.NoError(t, clientcmd.WriteToFile(*config, path))
	}

	dir := t.TempDir()
	ambient := filepath.Join(dir, "ambient")
	typed := filepath.Join(dir, "typed")
	writeKubeconfig(ambient, "https://ambient.invalid")
	writeKubeconfig(typed, "https://typed.invalid")

	// the deferred loader reads its kubeconfig once and keeps it, as the real clientConfig does
	rules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: ambient}
	restoreConfig := clientConfig
	clientConfig = &namespacedClientConfig{
		base: clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, &clientcmd.ConfigOverrides{}),
	}
	defer func() { clientConfig = restoreConfig }()

	RootCmd.PersistentPreRun(&cobra.Command{Use: cobra.ShellCompRequestCmd}, nil)

	// cobra parses a completion's flags only after the hook, so this is when a typed --kubeconfig lands
	rules.ExplicitPath = typed

	config, err := clientConfig.ClientConfig()
	require.NoError(t, err)
	assert.Equal(t, "https://typed.invalid", config.Host)
}

func Test_trimGoVersionFromRookVersion(t *testing.T) {
	type args struct {
		rookVersion string
	}

	sampleRookVersion := `rook: v1.11.0-alpha.0.420.gd9a17691c
go: go1.19.10s`
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "trim go version from rook version",
			args: args{rookVersion: sampleRookVersion},
			want: "rook: v1.11.0-alpha.0.420.gd9a17691c",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := trimGoVersionFromRookVersion(tt.args.rookVersion); got != tt.want {
				t.Errorf("trimGoVersionFromRookVersion() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConsumerContextFlag(t *testing.T) {
	flag := RootCmd.PersistentFlags().Lookup("consumer-context")
	if flag == nil {
		t.Fatal("expected --consumer-context flag to be registered")
	}
	if flag.DefValue != "" {
		t.Errorf("expected default value \"\", got %q", flag.DefValue)
	}
}
