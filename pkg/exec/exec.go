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

package exec

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"

	"github.com/rook/kubectl-rook-ceph/pkg/k8sutil"
	log "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
)

var (
	OperatorNamespace    string // operator namespae
	CephClusterNamespace string // Cephcluster namespace
)

func RunCommandInOperatorPod(ctx *k8sutil.Context, cmd string, args []string, operatorNamespace, clusterNamespace string) string {

	pod, err := k8sutil.WaitForOperatorPod(ctx, operatorNamespace)
	if err != nil {
		os.Exit(1)
	}

	output := new(bytes.Buffer)

	ExecCmdInPod(ctx, cmd, pod.Name, "rook-ceph-operator", pod.Namespace, clusterNamespace, args, output)
	return output.String()
}

func RunShellCommandInOperatorPod(ctx *k8sutil.Context, arg []string, operatorNamespace, clusterNamespace string) string {
	pod, err := k8sutil.WaitForOperatorPod(ctx, operatorNamespace)
	if err != nil {
		os.Exit(1)
	}

	cmd := "/bin/sh"
	args := []string{"-c"}
	args = append(args, arg...)

	output := new(bytes.Buffer)

	ExecCmdInPod(ctx, cmd, pod.Name, "rook-ceph-operator", pod.Namespace, clusterNamespace, args, output)
	return output.String()
}

// ExecCmdInPod exec command on specific pod and wait the command's output.
func ExecCmdInPod(ctx *k8sutil.Context, command, podName, containerName, podNamespace, clusterNamespace string, args []string, stdout io.Writer) {

	cmd := []string{}
	cmd = append(cmd, command)
	cmd = append(cmd, args...)

	if cmd[0] == "ceph" {
		cmd = append(cmd, "--connect-timeout=10", fmt.Sprintf("--conf=/var/lib/rook/%s/%s.config", clusterNamespace, clusterNamespace))
	} else if cmd[0] == "rbd" {
		cmd = append(cmd, fmt.Sprintf("--conf=/var/lib/rook/%s/%s.config", clusterNamespace, clusterNamespace))
	}

	// Prepare the API URL used to execute another process within the Pod.  In
	// this case, we'll run a remote shell.
	req := ctx.Clientset.CoreV1().RESTClient().
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
			TTY:       false,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(ctx.KubeConfig, "POST", req.URL())
	if err != nil {
		log.Fatal(err)
	}

	// Connect this process' std{in,out,err} to the remote shell process.
	err = exec.StreamWithContext(context.TODO(), remotecommand.StreamOptions{
		Stdin:  os.Stdin,
		Stdout: stdout,
		Stderr: os.Stderr,
		Tty:    false,
	})
	if err != nil {
		log.Fatal(err)
	}
}
