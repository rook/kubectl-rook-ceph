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
	"log"
	"time"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func RestartDeployment(namespace, deploymentName string) {
	// Get Kubernetes Client
	_, clientset, _ := GetKubeClient()

	deploymentsClient := clientset.AppsV1().Deployments(namespace)
	data := fmt.Sprintf(`{"spec": {"template": {"metadata": {"annotations": {"kubectl.kubernetes.io/restartedAt": "%s"}}}}}`, time.Now().String())
	_, err := deploymentsClient.Patch(context.TODO(), deploymentName, types.StrategicMergePatchType, []byte(data), v1.PatchOptions{})
	if err != nil {
		log.Fatalf("Failed to delete deployment %s: %v", deploymentName, err)
	}

	fmt.Printf("deployment.apps/%s restarted\n", deploymentName)
}

func UpdateConfigMap(namespace, configMapName, key, value string) {
	// Get Kubernetes Client
	_, clientset, _ := GetKubeClient()

	cm, err := clientset.CoreV1().ConfigMaps(namespace).Get(context.TODO(), configMapName, v1.GetOptions{})
	if err != nil {
		log.Fatal(err)
	}

	cm.Data[key] = value
	_, err = clientset.CoreV1().ConfigMaps(namespace).Update(context.TODO(), cm, v1.UpdateOptions{})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("configmap/%s patched\n", configMapName)
}
