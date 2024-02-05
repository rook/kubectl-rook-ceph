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

package restore

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"
	"github.com/rook/kubectl-rook-ceph/pkg/crds"
	"github.com/rook/kubectl-rook-ceph/pkg/k8sutil"
	"github.com/rook/kubectl-rook-ceph/pkg/logging"
	"github.com/rook/kubectl-rook-ceph/pkg/mons"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
)

func RestoreCrd(ctx context.Context, k8sclientset *k8sutil.Clientsets, operatorNamespace, clusterNamespace string, args []string) {
	crd := args[0]

	var crName string
	var crdResource unstructured.Unstructured
	if len(args) == 2 {
		crName = args[1]
	}

	logging.Info("Detecting which resources to restore for crd %q", crd)
	crdList, err := k8sclientset.ListResourcesDynamically(ctx, crds.CephRookIoGroup, crds.CephRookResourcesVersion, crd, clusterNamespace)
	if err != nil {
		logging.Fatal(fmt.Errorf("Failed to list resources for crd %v", err))
	}
	if len(crdList) == 0 {
		logging.Info("No Ceph CRDs found to restore")
		return
	}

	for _, cr := range crdList {
		if cr.GetDeletionTimestamp() != nil && (crName == "" || crName == cr.GetName()) {
			crName = cr.GetName()
			crdResource = *cr.DeepCopy()
			break
		}
	}

	if crName == "" {
		logging.Info("Nothing to do here, no %q resources in deleted state", crd)
		return
	}

	logging.Info("Restoring CR %s", crName)
	var answer string
	logging.Warning("The resource %s was found deleted. Do you want to restore it? yes | no\n", crName)
	fmt.Scanf("%s", &answer)
	err = mons.PromptToContinueOrCancel("yes", answer)
	if err != nil {
		logging.Fatal(fmt.Errorf("Restoring the resource %s cancelled", crName))
	}
	logging.Info("Proceeding with restoring deleting CR")

	logging.Info("Scaling down the operator")
	err = k8sutil.SetDeploymentScale(ctx, k8sclientset.Kube, operatorNamespace, "rook-ceph-operator", 0)
	if err != nil {
		deploy, er := k8sutil.GetDeployment(ctx, k8sclientset.Kube, operatorNamespace, "rook-ceph-operator")
		if er != nil && !apierrors.IsNotFound(er) {
			logging.Fatal(er)
		}
		if deploy.Status.ReadyReplicas != 0 {
			logging.Fatal(fmt.Errorf("Failed to scale down the operator deployment. %v", err))
		}
	}

	webhookConfigName := "rook-ceph-webhook"
	logging.Info("Deleting validating webhook %s if present", webhookConfigName)
	err = k8sclientset.Kube.AdmissionregistrationV1().ValidatingWebhookConfigurations().Delete(ctx, webhookConfigName, v1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		logging.Fatal(fmt.Errorf("failed to delete validating webhook %s. %v", webhookConfigName, err))
	}

	removeOwnerRefOfUID(ctx, k8sclientset, operatorNamespace, clusterNamespace, string(crdResource.GetUID()))

	logging.Info("Removing finalizers from %s/%s", crd, crName)

	jsonPatchData, _ := json.Marshal(crds.DefaultResourceRemoveFinalizers)
	err = k8sclientset.PatchResourcesDynamically(ctx, crds.CephRookIoGroup, crds.CephRookResourcesVersion, crd, clusterNamespace, crName, types.MergePatchType, jsonPatchData)
	if err != nil {
		logging.Fatal(fmt.Errorf("Failed to update resource %q for crd. %v", crName, err))
	}

	crdResource.SetResourceVersion("")
	crdResource.SetUID("")
	crdResource.SetSelfLink("")
	crdResource.SetCreationTimestamp(v1.Time{})
	logging.Info("Re-creating the CR %s from dynamic resource", crd)
	_, err = k8sclientset.CreateResourcesDynamically(ctx, crds.CephRookIoGroup, crds.CephRookResourcesVersion, crd, &crdResource, clusterNamespace)
	if err != nil {
		logging.Fatal((fmt.Errorf("Failed to create updated resource %q for crd. %v", crName, err)))
	}

	logging.Info("Scaling up the operator")
	err = k8sutil.SetDeploymentScale(ctx, k8sclientset.Kube, operatorNamespace, "rook-ceph-operator", 1)
	if err != nil {
		logging.Fatal(errors.Wrapf(err, "Operator pod still being scaled up"))
	}

	logging.Info("CR is successfully restored. Please watch the operator logs and check the crd")
}

func removeOwnerRefOfUID(ctx context.Context, k8sclientset *k8sutil.Clientsets, operatorNamespace, clusterNamespace, targetUID string) {
	logging.Info("Removing ownerreferences from resources with matching uid %s", targetUID)

	secrets, err := k8sclientset.Kube.CoreV1().Secrets(clusterNamespace).List(ctx, v1.ListOptions{})
	if err != nil {
		logging.Fatal(errors.Wrapf(err, "Failed to list secrets"))
	}

	for _, secret := range secrets.Items {
		for _, owner := range secret.OwnerReferences {
			if string(owner.UID) == targetUID {
				logging.Info("Removing owner references for secret %s", secret.Name)
				// Remove ownerReferences.
				secret.OwnerReferences = nil

				// Update the Secret without ownerReferences.
				_, err := k8sclientset.Kube.CoreV1().Secrets(clusterNamespace).Update(ctx, &secret, v1.UpdateOptions{})
				if err != nil {
					logging.Fatal(errors.Wrapf(err, "Failed to update ownerReferences for secret %s", secret.Name))
				}
				logging.Info("Removed ownerReference for Secret: %s\n", secret.Name)
			}
		}
	}

	cms, err := k8sclientset.Kube.CoreV1().ConfigMaps(clusterNamespace).List(ctx, v1.ListOptions{})
	if err != nil {
		logging.Fatal(errors.Wrapf(err, "Failed to list configMaps"))
	}

	for _, cm := range cms.Items {
		for _, owner := range cm.OwnerReferences {
			if string(owner.UID) == targetUID {
				logging.Info("Removing owner references for configmaps %s", cm.Name)
				// Remove ownerReferences.
				cm.OwnerReferences = nil

				// Update the Secret without ownerReferences.
				_, err := k8sclientset.Kube.CoreV1().ConfigMaps(clusterNamespace).Update(ctx, &cm, v1.UpdateOptions{})
				if err != nil {
					logging.Fatal(errors.Wrapf(err, "Failed to update ownerReferences for configmaps %s", cm.Name))
				}
				logging.Info("Removed ownerReference for configmap: %s\n", cm.Name)
			}
		}
	}

	services, err := k8sclientset.Kube.CoreV1().Services(clusterNamespace).List(ctx, v1.ListOptions{})
	if err != nil {
		logging.Fatal(errors.Wrapf(err, "Failed to list services"))
	}

	for _, service := range services.Items {
		for _, owner := range service.OwnerReferences {
			if string(owner.UID) == targetUID {
				logging.Info("Removing owner references for service %s", service.Name)
				// Remove ownerReferences.
				service.OwnerReferences = nil

				// Update the Secret without ownerReferences.
				_, err := k8sclientset.Kube.CoreV1().Services(clusterNamespace).Update(ctx, &service, v1.UpdateOptions{})
				if err != nil {
					logging.Fatal(errors.Wrapf(err, "Failed to update ownerReferences for service %s", service.Name))
				}
				logging.Info("Removed ownerReference for service: %s\n", service.Name)
			}
		}
	}

	deploys, err := k8sclientset.Kube.AppsV1().Deployments(clusterNamespace).List(ctx, v1.ListOptions{})
	if err != nil {
		logging.Fatal(errors.Wrapf(err, "Failed to list deployments"))
	}

	for _, deploy := range deploys.Items {
		for _, owner := range deploy.OwnerReferences {
			if string(owner.UID) == targetUID {
				logging.Info("Removing owner references for deployment %s", deploy.Name)
				// Remove ownerReferences.
				deploy.OwnerReferences = nil

				// Update the Secret without ownerReferences.
				_, err := k8sclientset.Kube.AppsV1().Deployments(clusterNamespace).Update(ctx, &deploy, v1.UpdateOptions{})
				if err != nil {
					logging.Fatal(errors.Wrapf(err, "Failed to update ownerReferences for deployemt %s", deploy.Name))
				}
				logging.Info("Removed ownerReference for deployment: %s\n", deploy.Name)
			}
		}
	}

	pvcs, err := k8sclientset.Kube.CoreV1().PersistentVolumeClaims(clusterNamespace).List(ctx, v1.ListOptions{})
	if err != nil {
		logging.Fatal(errors.Wrapf(err, "Failed to list pvc"))
	}

	for _, pvc := range pvcs.Items {
		for _, owner := range pvc.OwnerReferences {
			if string(owner.UID) == targetUID {
				logging.Info("Removing owner references for pvc %s", pvc.Name)
				// Remove ownerReferences.
				pvc.OwnerReferences = nil

				// Update the Secret without ownerReferences.
				_, err := k8sclientset.Kube.CoreV1().PersistentVolumeClaims(clusterNamespace).Update(ctx, &pvc, v1.UpdateOptions{})
				if err != nil {
					logging.Fatal(errors.Wrapf(err, "Failed to update ownerReferences for pvc %s", pvc.Name))
				}
				logging.Info("Removed ownerReference for pvc: %s\n", pvc.Name)
			}
		}
	}
}
