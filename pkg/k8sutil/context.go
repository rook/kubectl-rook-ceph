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
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	rookclient "github.com/rook/rook/pkg/client/clientset/versioned"
)

type Clientsets struct {
	// The Kubernetes config used for these client sets
	KubeConfig *rest.Config

	// Kube is a connection to the core Kubernetes API
	Kube kubernetes.Interface

	// Rook is a typed connection to the rook API
	Rook rookclient.Interface

	// Dynamic is used for manage dynamic resources
	Dynamic dynamic.Interface
}
