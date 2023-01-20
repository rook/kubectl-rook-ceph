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
	log "github.com/sirupsen/logrus"
	k8s "k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func GetKubeClient() (*rest.Config, *k8s.Clientset, *corev1client.CoreV1Client) {
	// 1. Create Kubernetes Client
	kubeconfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(),
		&clientcmd.ConfigOverrides{},
	)
	rest, err := kubeconfig.ClientConfig()
	if err != nil {
		log.Fatal(err)
	}

	clientset, err := k8s.NewForConfig(rest)
	if err != nil {
		log.Fatal(err)
	}

	// Create a Kubernetes core/v1 client.
	client, err := corev1client.NewForConfig(rest)
	if err != nil {
		log.Fatal(err)
	}

	return rest, clientset, client
}
