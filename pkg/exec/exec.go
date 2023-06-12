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
	"github.com/rook/kubectl-rook-ceph/pkg/logging"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
)

var (
	OperatorNamespace    string // operator namespae
	CephClusterNamespace string // Cephcluster namespace
)

func RunCommandInOperatorPod(ctx context.Context, clientsets *k8sutil.Clientsets, cmd string, args []string, operatorNamespace, clusterNamespace string, returnOutput, exitOnError bool) string {
	var pod v1.Pod
	var err error

	pod, err = k8sutil.WaitForPodToRun(ctx, clientsets.Kube, operatorNamespace, "app=rook-ceph-operator")
	if err != nil {
		logging.Fatal(fmt.Errorf("failed to wait for operator pod to run: %v", err))
	}

	var stdout, stderr bytes.Buffer

	execCmdInPod(ctx, clientsets, cmd, pod.Name, "rook-ceph-operator", pod.Namespace, clusterNamespace, args, &stdout, &stderr, returnOutput, exitOnError)
	if !returnOutput {
		return ""
	}
	fmt.Println(stdout.String())

	fmt.Print(stderr.String())
	return stdout.String()
}

func RunCommandInToolboxPod(ctx context.Context, clientsets *k8sutil.Clientsets, cmd string, args []string, clusterNamespace string, returnOutput, exitOnError bool) string {
	var pod v1.Pod
	var err error

	pod, err = k8sutil.WaitForPodToRun(ctx, clientsets.Kube, clusterNamespace, "app=rook-ceph-tools")
	if err != nil {
		logging.Fatal(err)
	}

	var stdout, stderr bytes.Buffer

	execCmdInPod(ctx, clientsets, cmd, pod.Name, "rook-ceph-tools", pod.Namespace, clusterNamespace, args, &stdout, &stderr, returnOutput, exitOnError)
	if !returnOutput {
		return ""
	}

	fmt.Println(stderr.String())
	return stdout.String()
}

func RunCommandInLabeledPod(ctx context.Context, clientsets *k8sutil.Clientsets, label, container, cmd string, args []string, clusterNamespace string, returnOutput, exitOnError bool) string {
	var list *v1.PodList
	var err error

	opts := metav1.ListOptions{LabelSelector: label}
	list, err = clientsets.Kube.CoreV1().Pods(clusterNamespace).List(ctx, opts)
	if err != nil || len(list.Items) == 0 {
		logging.Fatal(fmt.Errorf("failed to get rook mon pod where the command could be executed. %v", err))
	}
	var stdout, stderr bytes.Buffer

	execCmdInPod(ctx, clientsets, cmd, list.Items[0].Name, container, list.Items[0].Namespace, clusterNamespace, args, &stdout, &stderr, returnOutput, exitOnError)
	if !returnOutput {
		return ""
	}

	logging.Info(stderr.String())
	return stdout.String()
}

// execCmdInPod exec command on specific pod and wait the command's output.
func execCmdInPod(ctx context.Context, clientsets *k8sutil.Clientsets,
	command, podName, containerName, podNamespace, clusterNamespace string,
	args []string, stdout, stderr io.Writer, returnOutput, exitOnError bool) {

	cmd := []string{}
	cmd = append(cmd, command)
	cmd = append(cmd, args...)

	if containerName == "rook-ceph-tools" {
		cmd = append(cmd, "--connect-timeout=10")
	} else if cmd[0] == "ceph" {
		cmd = append(cmd, "--connect-timeout=10", fmt.Sprintf("--conf=/var/lib/rook/%s/%s.config", clusterNamespace, clusterNamespace))
	} else if cmd[0] == "rbd" {
		cmd = append(cmd, fmt.Sprintf("--conf=/var/lib/rook/%s/%s.config", clusterNamespace, clusterNamespace))
	}

	// Prepare the API URL used to execute another process within the Pod.  In
	// this case, we'll run a remote shell.
	req := clientsets.Kube.CoreV1().RESTClient().
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

	exec, err := remotecommand.NewSPDYExecutor(clientsets.KubeConfig, "POST", req.URL())
	if err != nil {
		logging.Fatal(err)
	}

	// returnOutput is true, the command's output will be print on shell directly with os.Stdout or os.Stderr
	if !returnOutput {
		// Connect this process' std{in,out,err} to the remote shell process.
		err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
			Stdin:  os.Stdin,
			Stdout: os.Stdout,
			Stderr: os.Stderr,
			Tty:    false,
		})
		if err != nil {
			logging.Error(err)
			if exitOnError {
				os.Exit(1)
			}
		}
		return
	} else {
		// Connect this process' std{in,out,err} to the remote shell process.
		err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
			Stdin:  os.Stdin,
			Stdout: stdout,
			Stderr: stderr,
			Tty:    false,
		})
		if err != nil {
			logging.Error(err)
			if exitOnError {
				os.Exit(1)
			}
		}
	}
}
