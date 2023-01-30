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

package debug

import (
	ctx "context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/rook/kubectl-rook-ceph/pkg/k8sutil"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func StartDebug(context *k8sutil.Context, clusterNamespace, deploymentName, alternateImageValue string) {

	err := startDebug(context, clusterNamespace, deploymentName, alternateImageValue)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func startDebug(context *k8sutil.Context, clusterNamespace, deploymentName, alternateImageValue string) error {
	deployment, err := verifyDeploymentExists(context, clusterNamespace, deploymentName)
	if err != nil {
		return fmt.Errorf("Missing mon or osd deployment name %s. %v\n", deploymentName, err)
	}

	if alternateImageValue != "" {
		log.Printf("setting debug image to %s\n", alternateImageValue)
		deployment.Spec.Template.Spec.Containers[0].Image = alternateImageValue
	}

	labels := deployment.Labels
	labels["ceph.rook.io/do-not-reconcile"] = "true"

	deployment.Spec.Template.Spec.Containers[0].LivenessProbe = nil
	deployment.Spec.Template.Spec.Containers[0].StartupProbe = nil

	fmt.Println("setting debug command to main container")

	deployment.Spec.Template.Spec.Containers[0].Command = []string{"sleep", "infinity"}
	deployment.Spec.Template.Spec.Containers[0].Args = []string{}

	if err := updateDeployment(context, clusterNamespace, deployment); err != nil {
		return fmt.Errorf("Failed to update deployment %s. %v\n", deployment.Name, err)
	}

	deploymentPodName, err := waitForPodToRun(context, clusterNamespace, deployment.Spec)
	if err != nil {
		fmt.Println(err)
		return err
	}

	if err := setDeploymentScale(context, clusterNamespace, deployment.Name, 0); err != nil {
		return err
	}

	fmt.Printf("waiting for the deployment pod %s to be deleted\n", deploymentPodName)

	err = waitForPodDeletion(context, clusterNamespace, deploymentPodName)
	if err != nil {
		fmt.Println(err)
		return err
	}

	debugDeploymentSpec := &appsv1.Deployment{
		ObjectMeta: v1.ObjectMeta{
			Name:      fmt.Sprintf("%s-debug", deploymentName),
			Namespace: clusterNamespace,
			Labels:    labels,
		},
		Spec: deployment.Spec,
	}

	debugDeployment, err := context.Clientset.AppsV1().Deployments(clusterNamespace).Create(ctx.TODO(), debugDeploymentSpec, v1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("Error creating deployment %s. %v\n", debugDeploymentSpec, err)
	}
	fmt.Printf("ensure the debug deployment %s is scaled up\n", deploymentName)

	if err := setDeploymentScale(context, clusterNamespace, debugDeployment.Name, 1); err != nil {
		return err
	}
	return nil
}

func setDeploymentScale(context *k8sutil.Context, clusterNamespace, deploymentName string, scaleCount int) error {
	scale := &autoscalingv1.Scale{
		ObjectMeta: v1.ObjectMeta{
			Name:      deploymentName,
			Namespace: clusterNamespace,
		},
		Spec: autoscalingv1.ScaleSpec{
			Replicas: int32(scaleCount),
		},
	}
	_, err := context.Clientset.AppsV1().Deployments(clusterNamespace).UpdateScale(ctx.TODO(), deploymentName, scale, v1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update scale of deployment %s. %v\n", deploymentName, err)
	}
	return nil
}

func verifyDeploymentExists(context *k8sutil.Context, clusterNamespace, deploymentName string) (*appsv1.Deployment, error) {
	deployment, err := context.Clientset.AppsV1().Deployments(clusterNamespace).Get(ctx.TODO(), deploymentName, v1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return deployment, nil
}

func updateDeployment(context *k8sutil.Context, clusterNamespace string, deploymentName *appsv1.Deployment) error {
	_, err := context.Clientset.AppsV1().Deployments(clusterNamespace).Update(ctx.TODO(), deploymentName, v1.UpdateOptions{})
	if err != nil {
		return err
	}
	return nil
}

func waitForPodToRun(context *k8sutil.Context, clusterNamespace string, deploymentSpec appsv1.DeploymentSpec) (string, error) {
	labelSelector := fmt.Sprintf("ceph_daemon_type=%s,ceph_daemon_id=%s", deploymentSpec.Template.Labels["ceph_daemon_type"], deploymentSpec.Template.Labels["ceph_daemon_id"])
	for i := 0; i < 60; i++ {
		pod, _ := context.Clientset.CoreV1().Pods(clusterNamespace).List(ctx.TODO(), v1.ListOptions{LabelSelector: labelSelector})
		if pod.Items[0].Status.Phase == corev1.PodRunning && pod.Items[0].DeletionTimestamp.IsZero() {
			return pod.Items[0].Name, nil
		}

		fmt.Println("waiting for pod to be running")
		time.Sleep(time.Second * 5)
	}

	return "", fmt.Errorf("No pod with labels matching %s:%s", deploymentSpec.Template.Labels, deploymentSpec.Template.Labels)

}

func waitForPodDeletion(context *k8sutil.Context, clusterNamespace, podName string) error {
	for i := 0; i < 60; i++ {
		_, err := context.Clientset.CoreV1().Pods(clusterNamespace).Get(ctx.TODO(), podName, v1.GetOptions{})
		if kerrors.IsNotFound(err) {
			return nil
		}

		fmt.Printf("waiting for pod %q to be deleted\n", podName)
		time.Sleep(time.Second * 5)
	}

	return fmt.Errorf("failed to delete pod %s", podName)
}
