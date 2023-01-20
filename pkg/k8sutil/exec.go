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

package k8sutil

import (
	"context"
	"fmt"
	"os"
	"strings"

	log "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

var (
	OperatorNamespace    string // operator namespae
	CephClusterNamespace string // Cephcluster namespace
)

func RunCommandInOperatorPod(cmd string, args []string, operatorNamespace, clusterNamespace string) {

	// Get Kubernetes Client
	rest, _, client := GetKubeClient()

	opts := metav1.ListOptions{LabelSelector: fmt.Sprintf("app=%s", "rook-ceph-operator")}
	list, err := client.Pods(operatorNamespace).List(context.TODO(), opts)
	if err != nil || len(list.Items) == 0 {
		log.Error("failed to get rook operator pod where the command could be executed")
		log.Fatal(err)
	}

	ExecCmd(rest, cmd, list.Items[0].Name, "rook-ceph-operator", list.Items[0].Namespace, clusterNamespace, args)
}

// ExecCmd exec command on specific pod and wait the command's output.
func ExecCmd(restconfig *restclient.Config, command, podName, containerName, podNamespace, clusterNamespace string, args []string) {
	// Create a Kubernetes core/v1 client.
	coreclient, err := corev1client.NewForConfig(restconfig)
	if err != nil {
		log.Fatal(err)
	}
	cmd := []string{}
	cmd = append(cmd, command)
	cmd = append(cmd, args...)

	if cmd[0] == "ceph" {
		cmd = append(cmd, "--connect-timeout=10")
	}

	cmd = append(cmd, fmt.Sprintf("--conf=/var/lib/rook/%s/%s.config", clusterNamespace, clusterNamespace))

	fmt.Printf("\n%s\n", strings.Join(cmd, " "))

	// Prepare the API URL used to execute another process within the Pod.  In
	// this case, we'll run a remote shell.
	req := coreclient.RESTClient().
		Post().
		Namespace(podNamespace).
		Resource("pods").
		Name(podName).
		SubResource("exec").
		VersionedParams(&v1.PodExecOptions{
			Container: containerName,
			Command:   cmd,
			Stdin:     true,
			Stdout:    true,
			Stderr:    true,
			TTY:       true,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(restconfig, "POST", req.URL())
	if err != nil {
		log.Fatal(err)
	}

	// Connect this process' std{in,out,err} to the remote shell process.
	err = exec.StreamWithContext(context.TODO(), remotecommand.StreamOptions{
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Tty:    true,
	})
	if err != nil {
		log.Fatal(err)
	}
}
