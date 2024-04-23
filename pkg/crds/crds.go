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

package crds

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/pkg/errors"
	"github.com/rook/kubectl-rook-ceph/pkg/k8sutil"
	"github.com/rook/kubectl-rook-ceph/pkg/logging"
	corev1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
)

var CephResources = []string{
	"cephclusters",
	"cephblockpoolradosnamespaces",
	"cephblockpools",
	"cephbucketnotifications",
	"cephbuckettopics",
	"cephclients",
	"cephcosidrivers",
	"cephfilesystemmirrors",
	"cephfilesystems",
	"cephfilesystemsubvolumegroups",
	"cephnfses",
	"cephobjectrealms",
	"cephobjectstores",
	"cephobjectstoreusers",
	"cephobjectzonegroups",
	"cephobjectzones",
	"cephrbdmirrors",
}

const (
	CephRookIoGroup          = "ceph.rook.io"
	CephRookResourcesVersion = "v1"
)

const (
	CephResourceCephClusters = "cephclusters"
	toolBoxDeployment        = "rook-ceph-tools"
	timeOutCheckResources    = 5 * time.Second
	maxExecutionTime         = 15 * time.Minute
	maxDisplayedPods         = 10
)

var (
	clusterResourcePatchFinalizer = map[string]interface{}{
		"spec": map[string]interface{}{
			"cleanupPolicy": map[string]string{
				"confirmation": "yes-really-destroy-data",
			},
		},
	}

	DefaultResourceRemoveFinalizers = map[string]interface{}{
		"metadata": map[string]interface{}{
			"finalizers": nil,
		},
	}
)

func DeleteCustomResources(ctx context.Context, clientsets k8sutil.ClientsetsInterface, k8sClientSet kubernetes.Interface, clusterNamespace string) {
	err := deleteCustomResources(ctx, clientsets, clusterNamespace)
	if err != nil {
		logging.Fatal(err)
	}
	k8sutil.DeleteDeployment(ctx, k8sClientSet, clusterNamespace, toolBoxDeployment)
	ensureClusterIsEmpty(ctx, k8sClientSet, clusterNamespace)
	logging.Info("done")
}

func deleteCustomResources(ctx context.Context, clientsets k8sutil.ClientsetsInterface, clusterNamespace string) error {
	for _, resource := range CephResources {
		logging.Info("getting resource kind %s", resource)
		items, err := clientsets.ListResourcesDynamically(ctx, CephRookIoGroup, CephRookResourcesVersion, resource, clusterNamespace)
		if err != nil {
			if k8sErrors.IsNotFound(err) {
				logging.Info("the server could not find the requested resource: %s", resource)
				continue
			}
			return err
		}

		if len(items) == 0 {
			logging.Info("resource %s was not found on the cluster", resource)
			continue
		}

		for _, item := range items {
			logging.Info(fmt.Sprintf("removing resource %s: %s", resource, item.GetName()))
			err = clientsets.DeleteResourcesDynamically(ctx, CephRookIoGroup, CephRookResourcesVersion, resource, clusterNamespace, item.GetName())
			if err != nil {
				if k8sErrors.IsNotFound(err) {
					logging.Info(err.Error())
					continue
				}
				return err
			}

			itemResource, err := clientsets.GetResourcesDynamically(ctx, CephRookIoGroup, CephRookResourcesVersion, resource, item.GetName(), clusterNamespace)
			if err != nil {
				if !k8sErrors.IsNotFound(err) {
					return err
				}
			}

			if itemResource != nil {
				logging.Info(fmt.Sprintf("resource %q is not yet deleted, applying patch to remove finalizer...", itemResource.GetName()))
				err = updatingFinalizers(ctx, clientsets, itemResource, resource, clusterNamespace)
				if err != nil {
					if k8sErrors.IsNotFound(err) {
						logging.Info(err.Error())
						continue
					}
					return err
				}

				err = clientsets.DeleteResourcesDynamically(ctx, CephRookIoGroup, CephRookResourcesVersion, resource, clusterNamespace, item.GetName())
				if err != nil {
					if !k8sErrors.IsNotFound(err) {
						return err
					}
				}
			}

			itemResource, err = clientsets.GetResourcesDynamically(ctx, CephRookIoGroup, CephRookResourcesVersion, resource, item.GetName(), clusterNamespace)
			if err != nil {
				if !k8sErrors.IsNotFound(err) {
					return err
				}
			}

			logging.Info("resource %s was deleted", item.GetName())
		}
	}
	return nil
}

func updatingFinalizers(ctx context.Context, clientsets k8sutil.ClientsetsInterface, itemResource *unstructured.Unstructured, resource, clusterNamespace string) error {
	if resource == CephResourceCephClusters {
		jsonPatchData, _ := json.Marshal(clusterResourcePatchFinalizer)
		err := clientsets.PatchResourcesDynamically(ctx, CephRookIoGroup, CephRookResourcesVersion, resource, clusterNamespace, itemResource.GetName(), types.MergePatchType, jsonPatchData)
		if err != nil {
			return err
		}
		logging.Info("Added cleanup policy to the cephcluster CR %q", resource)
		return nil
	}

	jsonPatchData, _ := json.Marshal(DefaultResourceRemoveFinalizers)
	err := clientsets.PatchResourcesDynamically(ctx, CephRookIoGroup, CephRookResourcesVersion, resource, clusterNamespace, itemResource.GetName(), types.MergePatchType, jsonPatchData)
	if err != nil {
		return err
	}

	return nil
}

func ensureClusterIsEmpty(ctx context.Context, k8sClientSet kubernetes.Interface, clusterNamespace string) {
	logging.Info("waiting to clean up resources")
	ctx, cancel := context.WithTimeout(context.Background(), maxExecutionTime)
	defer cancel()

	for {
		select {
		case <-time.After(timeOutCheckResources):
			pods, err := k8sClientSet.CoreV1().Pods(clusterNamespace).List(ctx, v1.ListOptions{LabelSelector: "rook_cluster=" + clusterNamespace})
			if err != nil {
				logging.Fatal(err)
			}

			if len(pods.Items) == 0 {
				return
			}

			logging.Info("%d pods still alive, removing....", len(pods.Items))
			for i, pod := range pods.Items {
				if i < maxDisplayedPods {
					logging.Info("pod %s still alive", pod.Name)
				}
				appLabel := getPodLabel(ctx, pod, "app")
				err := pruneDeployments(ctx, k8sClientSet, clusterNamespace, appLabel)
				if err != nil {
					logging.Warning("failed to prune deployments. %v", err)
				}
				err = pruneJobs(ctx, k8sClientSet, clusterNamespace, appLabel)
				if err != nil {
					logging.Warning("failed to prune jobs. %v", err)
				}
			}

		case <-ctx.Done():
			logging.Info("Timeout reached, exiting cleanup loop")
			return
		}
	}
}

func getPodLabel(_ context.Context, pod corev1.Pod, label string) string {
	for podLabelName, podLabelValue := range pod.Labels {
		if podLabelName == label {
			return podLabelValue
		}
	}
	return ""
}

func pruneDeployments(ctx context.Context, k8sClientSet kubernetes.Interface, clusterNamespace, labelApp string) error {
	deployments, err := k8sClientSet.
		AppsV1().
		Deployments(clusterNamespace).List(ctx, v1.ListOptions{
		LabelSelector: "rook_cluster=" + clusterNamespace + ",app=" + labelApp,
	})
	if err != nil {
		if !k8sErrors.IsNotFound(err) {
			return err
		}
		return nil
	}

	for _, deployment := range deployments.Items {
		logging.Info("deployment %s exists removing....", deployment.Name)
		k8sutil.DeleteDeployment(ctx, k8sClientSet, clusterNamespace, deployment.Name)
	}
	return nil
}

func pruneJobs(ctx context.Context, k8sClientSet kubernetes.Interface, clusterNamespace, labelApp string) error {
	selector := "rook_cluster=" + clusterNamespace + ",app=" + labelApp
	jobs, err := k8sClientSet.BatchV1().Jobs(clusterNamespace).List(ctx, v1.ListOptions{
		LabelSelector: selector,
	})
	if err != nil {
		if !k8sErrors.IsNotFound(err) {
			return err
		}
		return nil
	}

	for _, job := range jobs.Items {
		logging.Info("job %s exists removing....", job.Name)
		var gracePeriod int64
		propagation := metav1.DeletePropagationForeground
		options := &metav1.DeleteOptions{GracePeriodSeconds: &gracePeriod, PropagationPolicy: &propagation}
		if err := k8sClientSet.BatchV1().Jobs(clusterNamespace).Delete(ctx, job.Name, *options); err != nil {
			if !k8sErrors.IsNotFound(err) {
				return errors.Wrapf(err, "failed to delete job %q", job.Name)
			}
		}
	}
	return nil
}
