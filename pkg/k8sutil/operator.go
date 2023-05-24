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
	"k8s.io/client-go/kubernetes"
)

func RestartDeployment(ctx context.Context, k8sclientset kubernetes.Interface, namespace, deploymentName string) {
	deploymentsClient := k8sclientset.AppsV1().Deployments(namespace)
	data := fmt.Sprintf(`{"spec": {"template": {"metadata": {"annotations": {"kubectl.kubernetes.io/restartedAt": "%s"}}}}}`, time.Now().String())
	_, err := deploymentsClient.Patch(ctx, deploymentName, types.StrategicMergePatchType, []byte(data), v1.PatchOptions{})
	if err != nil {
		logging.Error(fmt.Errorf("Failed to delete deployment %s: %v", deploymentName, err))
	}

	logging.Info("deployment.apps/%s restarted\n", deploymentName)
}

func WaitForPodToRun(ctx context.Context, k8sclientset kubernetes.Interface, namespace, labelSelector string) (corev1.Pod, error) {
	opts := v1.ListOptions{LabelSelector: labelSelector}
	for i := 0; i < 60; i++ {
		pod, err := k8sclientset.CoreV1().Pods(namespace).List(ctx, opts)
		if err != nil {
			return corev1.Pod{}, fmt.Errorf("failed to list pods with labels matching %s", labelSelector)
		}
		if len(pod.Items) != 0 {
			if pod.Items[0].Status.Phase == corev1.PodRunning && pod.Items[0].DeletionTimestamp.IsZero() {
				return pod.Items[0], nil
			}
		}

		logging.Info("waiting for pod with label %q in namespace %q to be running", labelSelector, namespace)
		time.Sleep(time.Second * 5)
	}

	return corev1.Pod{}, fmt.Errorf("No pod with labels matching %s", labelSelector)
}

func UpdateConfigMap(ctx context.Context, k8sclientset kubernetes.Interface, namespace, configMapName, key, value string) {
	cm, err := k8sclientset.CoreV1().ConfigMaps(namespace).Get(ctx, configMapName, v1.GetOptions{})
	if err != nil {
		logging.Fatal(err)
	}

	cm.Data[key] = value
	_, err = k8sclientset.CoreV1().ConfigMaps(namespace).Update(ctx, cm, v1.UpdateOptions{})
	if err != nil {
		logging.Fatal(err)
	}

	logging.Info("configmap/%s patched\n", configMapName)
}
