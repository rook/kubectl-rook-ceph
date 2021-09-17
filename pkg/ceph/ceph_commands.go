/*
Copyright © 2021 kubectl-rook-ceph

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
package ceph

import (
	"context"
	"fmt"
	"kubectl-rook-ceph/pkg"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	//k8s "k8s.io/client-go/kubernetes"

	"k8s.io/client-go/kubernetes/scheme"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"

	//"k8s.io/client-go/rest"
	restclient "k8s.io/client-go/rest"

	log "github.com/sirupsen/logrus"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
)

func fail(msg string) {
	fmt.Printf(msg)
	os.Exit(1)
}

func CephCommands(cmd *cobra.Command, args []string) {
	var flagName []string
	var flagValue []string

	cmd.Flags().Parse(args)
	cmd.Flags().Visit(func(f *pflag.Flag) {
		flagName = append(flagName, f.Name)
		flagValue = append(flagValue, f.Value.String())
	})

	// 1. Create Kubernetes Client
	kubeconfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(),
		&clientcmd.ConfigOverrides{},
	)
	rest, err := kubeconfig.ClientConfig()
	if err != nil {
		log.Fatal(err)
	}

	// Get Kubernetes Client
	_, client := pkg.GetKubeClinet()

	opts := metav1.ListOptions{LabelSelector: fmt.Sprintf("app=%s", "rook-ceph-tools")}
	list, err := client.Pods("rook-ceph").List(context.TODO(), opts)

	if err != nil || len(list.Items) == 0 {
		opts := metav1.ListOptions{LabelSelector: fmt.Sprintf("app=%s", "rook-ceph-operator")}
		list, err = client.Pods("rook-ceph").List(context.TODO(), opts)
		if err != nil || len(list.Items) == 0 {
			log.Error("failed to get pod to exec into container")
			log.Fatal(err)
		}
	}

	get, err := client.Pods("rook-ceph").Get(context.TODO(), list.Items[0].Name, metav1.GetOptions{})
	if err != nil || len(get.Status.ContainerStatuses) == 0 {
		log.Error("failed to get container")
		log.Fatal(err)
	}

	ExecCmd(rest, list.Items[0].Name, get.Status.ContainerStatuses[0].Name, args, flagName, flagValue)
}

// ExecCmd exec command on specific pod and wait the command's output.
func ExecCmd(restconfig *restclient.Config, podName string, containerName string, args []string, flagName []string, flagValue []string) {
	// Create a Kubernetes core/v1 client.
	coreclient, err := corev1client.NewForConfig(restconfig)
	if err != nil {
		log.Fatal(err)
	}
	cmd := []string{
		"ceph",
	}
	cmd = append(cmd, args...)

	if len(flagName) != 0 {
		for i := range flagName {
			cmd = append(cmd, fmt.Sprint("--", flagName[i]), flagValue[i])
		}
	}

	fmt.Println(strings.Join(cmd, " "))
	fmt.Println()

	// Prepare the API URL used to execute another process within the Pod.  In
	// this case, we'll run a remote shell.
	req := coreclient.RESTClient().
		Post().
		Namespace("rook-ceph").
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
	err = exec.Stream(remotecommand.StreamOptions{
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Tty:    true,
	})
	if err != nil {
		log.Fatal(err)
	}
}
