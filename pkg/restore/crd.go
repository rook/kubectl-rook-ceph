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
	"fmt"
	"os"
	"strings"

	"github.com/pkg/errors"
	"github.com/rook/kubectl-rook-ceph/pkg/exec"
	"github.com/rook/kubectl-rook-ceph/pkg/k8sutil"
	"github.com/rook/kubectl-rook-ceph/pkg/logging"
	"github.com/rook/kubectl-rook-ceph/pkg/mons"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func RestoreCrd(ctx context.Context, k8sclientset *k8sutil.Clientsets, operatorNamespace, clusterNamespace string, args []string) {
	crd := args[0]
	logging.Info("Detecting which resources to restore for crd %q", crd)
	getCrName := `kubectl -n %s get %s -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.metadata.deletionGracePeriodSeconds}{"\n"}{end}' | awk '$2=="0" {print $1}' | head -n 1`
	command := fmt.Sprintf(getCrName, clusterNamespace, crd)
	crName := strings.TrimSpace(exec.ExecuteBashCommand(command))

	if crName == "" {
		logging.Info("Nothing to do here, no %q resources in deleted state", crd)
		return
	}

	if len(args) == 2 {
		crName = args[1]
	}

	logging.Info("Restoring CR %s", crName)
	var answer string
	logging.Warning("The resource %s was found deleted. Do you want to restore it? yes | no\n", crName)
	fmt.Scanf("%s", &answer)
	err := mons.PromptToContinueOrCancel("yes", answer)
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

	logging.Info("Backing up kubernetes and crd resources")
	crFileName := crd + "-" + crName + ".yaml"
	getCrYamlContent := `kubectl -n %s get %s %s -oyaml > %s`
	command = fmt.Sprintf(getCrYamlContent, clusterNamespace, crd, crName, crFileName)
	exec.ExecuteBashCommand(command)
	logging.Info("Backed up crd %s/%s in file %s", crd, crName, crFileName)

	webhookConfigName := "rook-ceph-webhook"
	logging.Info("Deleting validating webhook %s if present", webhookConfigName)
	err = k8sclientset.Kube.AdmissionregistrationV1().ValidatingWebhookConfigurations().Delete(ctx, webhookConfigName, v1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		logging.Fatal(fmt.Errorf("failed to delete validating webhook %s. %v", webhookConfigName, err))
	}

	logging.Info("Fetching the UID for %s/%s", crd, crName)
	getCrUID := `kubectl -n %s get %s %s -o 'jsonpath={.metadata.uid}'`
	command = fmt.Sprintf(getCrUID, clusterNamespace, crd, crName)
	uid := exec.ExecuteBashCommand(command)
	logging.Info("Successfully fetched uid %s from %s/%s", uid, crd, crName)

	removeOwnerRefOfUID(ctx, k8sclientset, operatorNamespace, clusterNamespace, uid)

	logging.Info("Removing finalizers from %s/%s", crd, crName)
	removeFinalizers := `kubectl -n %s patch %s/%s --type json --patch='[ { "op": "remove", "path": "/metadata/finalizers" } ]'`
	command = fmt.Sprintf(removeFinalizers, clusterNamespace, crd, crName)
	logging.Info(exec.ExecuteBashCommand(command))

	logging.Info("Re-creating the CR %s from file %s created above", crd, crFileName)
	recreateCR := `kubectl create -f %s`
	command = fmt.Sprintf(recreateCR, crFileName)
	logging.Info(exec.ExecuteBashCommand(command))

	logging.Info("Scaling up the operator")
	err = k8sutil.SetDeploymentScale(ctx, k8sclientset.Kube, operatorNamespace, "rook-ceph-operator", 1)
	if err != nil {
		logging.Fatal(errors.Wrapf(err, "Operator pod still being scaled up"))
	}

	if err = os.Remove(crFileName); err != nil {
		logging.Warning("Unable to remove. Please remove the file %s manually.%v", crFileName, err)
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
