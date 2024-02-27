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

package maintenance

import (
	"context"
	"fmt"
	"time"

	"github.com/rook/kubectl-rook-ceph/pkg/k8sutil"
	"github.com/rook/kubectl-rook-ceph/pkg/logging"

	appsv1 "k8s.io/api/apps/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func StartMaintenance(ctx context.Context, k8sclientset kubernetes.Interface, clusterNamespace, deploymentName, alternateImageValue string) {
	err := startmaintenance(ctx, k8sclientset, clusterNamespace, deploymentName, alternateImageValue)
	if err != nil {
		logging.Fatal(err)
	}
}

func startmaintenance(ctx context.Context, k8sclientset kubernetes.Interface, clusterNamespace, deploymentName, alternateImageValue string) error {
	originalDeployment, err := k8sutil.GetDeployment(ctx, k8sclientset, clusterNamespace, deploymentName)
	if err != nil {
		return fmt.Errorf("Missing mon or osd deployment name %s. %v\n", deploymentName, err)
	}

	// We need to dereference the deployment as it is required for the maintenance deployment
	deployment := *originalDeployment

	if alternateImageValue != "" {
		logging.Info("setting maintenance image to %s\n", alternateImageValue)
		deployment.Spec.Template.Spec.Containers[0].Image = alternateImageValue
	}

	labels := deployment.Labels
	labels["ceph.rook.io/do-not-reconcile"] = "true"

	deployment.Spec.Template.Spec.Containers[0].LivenessProbe = nil
	deployment.Spec.Template.Spec.Containers[0].StartupProbe = nil

	logging.Info("setting maintenance command to main container")

	deployment.Spec.Template.Spec.Containers[0].Command = []string{"sleep", "infinity"}
	deployment.Spec.Template.Spec.Containers[0].Args = []string{}

	labelSelector := fmt.Sprintf("ceph_daemon_type=%s,ceph_daemon_id=%s", deployment.Spec.Template.Labels["ceph_daemon_type"], deployment.Spec.Template.Labels["ceph_daemon_id"])
	deploymentPodName, err := k8sutil.WaitForPodToRun(ctx, k8sclientset, clusterNamespace, labelSelector)
	if err != nil {
		return err
	}

	if err := k8sutil.SetDeploymentScale(ctx, k8sclientset, clusterNamespace, deployment.Name, 0); err != nil {
		return err
	}

	logging.Info("deployment %s scaled down\n", deployment.Name)
	logging.Info("waiting for the deployment pod %s to be deleted\n", deploymentPodName.Name)

	err = waitForPodDeletion(ctx, k8sclientset, clusterNamespace, deploymentName)
	if err != nil {
		return err
	}

	maintenanceDeploymentSpec := &appsv1.Deployment{
		ObjectMeta: v1.ObjectMeta{
			Name:      fmt.Sprintf("%s-maintenance", deploymentName),
			Namespace: clusterNamespace,
			Labels:    labels,
		},
		Spec: deployment.Spec,
	}

	maintenanceDeployment, err := k8sclientset.AppsV1().Deployments(clusterNamespace).Create(ctx, maintenanceDeploymentSpec, v1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("Error creating deployment %s. %v\n", maintenanceDeploymentSpec, err)
	}
	logging.Info("ensure the maintenance deployment %s is scaled up\n", deploymentName)

	if err := k8sutil.SetDeploymentScale(ctx, k8sclientset, clusterNamespace, maintenanceDeployment.Name, 1); err != nil {
		return err
	}

	pod, err := k8sutil.WaitForPodToRun(ctx, k8sclientset, clusterNamespace, labelSelector)
	if err != nil {
		logging.Fatal(err)
	}

	logging.Info("pod %s is ready for maintenance operations", pod.Name)
	return nil
}

func waitForPodDeletion(ctx context.Context, k8sclientset kubernetes.Interface, clusterNamespace, podName string) error {
	for i := 0; i < 60; i++ {
		_, err := k8sclientset.CoreV1().Pods(clusterNamespace).Get(ctx, podName, v1.GetOptions{})
		if kerrors.IsNotFound(err) {
			return nil
		}

		logging.Info("waiting for pod %q to be deleted\n", podName)
		time.Sleep(time.Second * 5)
	}

	return fmt.Errorf("failed to delete pod %s", podName)
}
