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

package create

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"text/template"

	"github.com/rook/kubectl-rook-ceph/pkg/exec"
	"github.com/rook/kubectl-rook-ceph/pkg/k8sutil"
	"github.com/rook/kubectl-rook-ceph/pkg/logging"
)

type config struct {
	ClusterNamespace string
	StorageClassName string
}

var (
	githubURL = "https://raw.githubusercontent.com/rook/rook/master/deploy/examples/"
	//go:embed test-cluster-on-pvc.yaml
	clusterYaml string
)

func SampleCluster(ctx context.Context, clientsets *k8sutil.Clientsets, operatorNamespace, clusterNamespace, storageClassName string) {
	var answer, output string
	logging.Warning(`
	"This command will create a single-node Rook-Ceph cluster. We assume that you don't already have a Rook-Ceph cluster.
	We also assume that you have a Kubernetes cluster and have read this https://rook.github.io/docs/rook/latest/Contributing/development-environment/ . Please type 'yes' to proceed."
	`)

	fmt.Scanf("%s", &answer)
	output, err := printWarningMessage(answer)
	if err != nil {
		logging.Fatal(fmt.Errorf("Single node Rook-Ceph cluster creation aborted: %w", err))
	}
	logging.Info(output)

	crdsYaml, err := getFileContentFromGitHub(githubURL, "crds.yaml", operatorNamespace, clusterNamespace)
	if err != nil {
		logging.Error(fmt.Errorf("Error fetching file crds.yaml: %v", err))
		return
	}
	createK8sResources("crds.yaml", crdsYaml)

	commonYaml, err := getFileContentFromGitHub(githubURL, "common.yaml", operatorNamespace, clusterNamespace)
	if err != nil {
		logging.Error(fmt.Errorf("Error fetching file common.yaml: %v", err))
		return
	}
	createK8sResources("common.yaml", commonYaml)

	operatorYaml, err := getFileContentFromGitHub(githubURL, "operator.yaml", operatorNamespace, clusterNamespace)
	if err != nil {
		logging.Error(fmt.Errorf("Error fetching file operator.yaml: %v", err))
		return
	}
	createK8sResources("operator.yaml", operatorYaml)

	c := &config{
		ClusterNamespace: clusterNamespace,
		StorageClassName: storageClassName,
	}

	t, err := loadTemplate("test-cluster-on-pvc.yaml", clusterYaml, c)
	if err != nil {
		logging.Error(err)
	}
	createK8sResources("cluster-test.yaml", t)
}

func loadTemplate(name, templateFileText string, config interface{}) ([]byte, error) {
	var writer bytes.Buffer
	t := template.New(name)
	t, err := t.Parse(templateFileText)
	if err != nil {
		return nil, fmt.Errorf("failed to parse template %q: %w", name, err)
	}
	err = t.Execute(&writer, config)
	return writer.Bytes(), err
}

func createK8sResources(fileName string, t []byte) {
	// Create a temporary file
	tempFile, err := os.CreateTemp("", fileName)
	if err != nil {
		logging.Fatal(err)
	}

	// Write the string content to the file
	_, err = tempFile.WriteString(string(t))
	if err != nil {
		logging.Fatal(err)
	}

	// Close the file to ensure it's flushed to disk
	err = tempFile.Close()
	if err != nil {
		logging.Fatal(err)
	}

	logging.Info(exec.ExecuteBashCommand("kubectl apply -f " + tempFile.Name()))
}

func getFileContentFromGitHub(url, fileName, operatorNamespace, clusterNamespace string) ([]byte, error) {
	resp, err := http.Get(url + fileName)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch file fileName %s, status code: %d", fileName, resp.StatusCode)
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	content = []byte(strings.ReplaceAll(string(content), "rook-ceph # namespace:operator", operatorNamespace+" # namespace:operator"))
	content = []byte(strings.ReplaceAll(string(content), "rook-ceph # namespace:cluster", clusterNamespace+" # namespace:cluster"))
	return content, nil
}

func printWarningMessage(answer string) (string, error) {
	if skip, ok := os.LookupEnv("CREATE_SINGLE_NODE_CEPH_CLUSTER"); ok && skip == "true" {
		return "skipped prompt since CREATE_SINGLE_NODE_CEPH_CLUSTER=true", nil
	} else if answer == "yes" {
		return "proceeding to create single node ceph cluster", nil
	}

	return "", fmt.Errorf("cancelled")
}
