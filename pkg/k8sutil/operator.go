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
	"time"

	"github.com/rook/kubectl-rook-ceph/pkg/logging"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func RestartDeployment(ctx *Context, namespace, deploymentName string) {
	deploymentsClient := ctx.Clientset.AppsV1().Deployments(namespace)
	data := fmt.Sprintf(`{"spec": {"template": {"metadata": {"annotations": {"kubectl.kubernetes.io/restartedAt": "%s"}}}}}`, time.Now().String())
	_, err := deploymentsClient.Patch(context.TODO(), deploymentName, types.StrategicMergePatchType, []byte(data), v1.PatchOptions{})
	if err != nil {
		logging.Error(fmt.Errorf("Failed to delete deployment %s: %v", deploymentName, err))
	}

	logging.Info("deployment.apps/%s restarted\n", deploymentName)
}

func WaitForPodToRun(ctx *Context, operatorNamespace, labelSelector string) (corev1.Pod, error) {
	opts := v1.ListOptions{LabelSelector: labelSelector}
	for i := 0; i < 60; i++ {
		pod, err := ctx.Clientset.CoreV1().Pods(operatorNamespace).List(context.TODO(), opts)
		if err != nil {
			return corev1.Pod{}, fmt.Errorf("failed to list pods with labels matching %s", labelSelector)
		}
		if pod.Items[0].Status.Phase == corev1.PodRunning && pod.Items[0].DeletionTimestamp.IsZero() {
			return pod.Items[0], nil
		}

		logging.Info("waiting for pod to be running")
		time.Sleep(time.Second * 5)
	}

	return corev1.Pod{}, fmt.Errorf("No pod with labels matching %s", labelSelector)
}

func UpdateConfigMap(ctx *Context, namespace, configMapName, key, value string) {
	cm, err := ctx.Clientset.CoreV1().ConfigMaps(namespace).Get(context.TODO(), configMapName, v1.GetOptions{})
	if err != nil {
		logging.Fatal(err)
	}

	cm.Data[key] = value
	_, err = ctx.Clientset.CoreV1().ConfigMaps(namespace).Update(context.TODO(), cm, v1.UpdateOptions{})
	if err != nil {
		logging.Fatal(err)
	}

	logging.Info("configmap/%s patched\n", configMapName)
}
