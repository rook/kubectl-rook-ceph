/*
Copyright 2026 The Rook Authors. All rights reserved.

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

package health

import (
	"context"
	"fmt"
	"time"

	nbv1 "github.com/noobaa/noobaa-operator/v5/pkg/apis/noobaa/v1alpha1"
	"github.com/rook/kubectl-rook-ceph/pkg/logging"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

var noobaaGVR = schema.GroupVersionResource{
	Group:    "noobaa.io",
	Version:  "v1alpha1",
	Resource: "noobaas",
}

// checkNooBaaHealth checks the NooBaa CR phase and core pod status.
// Returns an empty CheckResult (Name == "") if no NooBaa resource exists.
func checkNooBaaHealth(ctx context.Context, dynamicClient dynamic.Interface, k8sclientset kubernetes.Interface, clusterNamespace string) CheckResult {
	phase, creationTime, found, err := getNooBaaPhase(ctx, dynamicClient, clusterNamespace)
	if !found {
		return CheckResult{}
	}
	logging.Plain("Checking %s...", CheckNooBaaHealth)
	result := CheckResult{
		Name:     CheckNooBaaHealth,
		Category: CategoryObjectStorage,
	}

	if err != nil {
		result.Status = StatusError
		result.Message = fmt.Sprintf("Failed to get NooBaa status: %v", err)
		return result
	}

	result.Status = StatusOK
	result.Message = fmt.Sprintf("NooBaa phase: %s", phase)

	switch nbv1.SystemPhase(phase) {
	case nbv1.SystemPhaseReady:
		result.Details = append(result.Details, "[INFO] Phase: Ready")
	case nbv1.SystemPhaseCreating, nbv1.SystemPhaseConnecting, nbv1.SystemPhaseConfiguring, nbv1.SystemPhaseVerifying:
		elapsed := time.Since(creationTime)
		if elapsed >= 5*time.Minute {
			result.Status = StatusWarning
			result.Details = append(result.Details, fmt.Sprintf("[WARN] Phase: %s (for %s)", phase, formatDuration(elapsed)))
		} else {
			result.Details = append(result.Details, fmt.Sprintf("[INFO] Phase: %s (for %s)", phase, formatDuration(elapsed)))
		}
	case nbv1.SystemPhaseRejected:
		result.Status = StatusCritical
		result.Details = append(result.Details, "[ERR] Phase: Rejected")
	default:
		result.Details = append(result.Details, fmt.Sprintf("[INFO] Phase: %s", phase))
	}

	pods, err := k8sclientset.CoreV1().Pods(clusterNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: "noobaa-core=noobaa",
	})
	if err != nil {
		result.Details = append(result.Details, fmt.Sprintf("[WARN] Failed to list NooBaa core pods: %v", err))
		result.Status = worseStatus(result.Status, StatusWarning)
		return result
	}

	if len(pods.Items) == 0 {
		result.Details = append(result.Details, "[WARN] No NooBaa core pods found")
		result.Status = worseStatus(result.Status, StatusWarning)
		return result
	}

	for i := range pods.Items {
		pod := &pods.Items[i]

		result.Items = append(result.Items, CheckItem{
			Name:   pod.Name,
			Status: string(pod.Status.Phase),
			Node:   pod.Spec.NodeName,
		})

		if containerName, crashing := isContainerCrashLooping(pod); crashing {
			result.Details = append(result.Details, fmt.Sprintf("[ERR] Core pod %s: container %s in CrashLoopBackOff", pod.Name, containerName))
			result.Status = worseStatus(result.Status, StatusCritical)
		} else if pod.Status.Phase == v1.PodPending {
			result.Details = append(result.Details, fmt.Sprintf("[WARN] Core pod %s is Pending", pod.Name))
			result.Status = worseStatus(result.Status, StatusCritical)
		} else {
			result.Details = append(result.Details, fmt.Sprintf("[INFO] Core pod %s is %s", pod.Name, pod.Status.Phase))
		}
	}

	return result
}

func getNooBaaPhase(ctx context.Context, dynamicClient dynamic.Interface, namespace string) (string, time.Time, bool, error) {
	list, err := dynamicClient.Resource(noobaaGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return "", time.Time{}, false, nil
	}

	if len(list.Items) == 0 {
		return "", time.Time{}, false, nil
	}

	obj := &list.Items[0]

	phase, _, err := unstructured.NestedString(obj.Object, "status", "phase")
	if err != nil {
		return "", time.Time{}, true, fmt.Errorf("failed to read NooBaa phase: %v", err)
	}

	creationTime := obj.GetCreationTimestamp().Time

	return phase, creationTime, true, nil
}

func isContainerCrashLooping(pod *v1.Pod) (string, bool) {
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.State.Waiting != nil && cs.State.Waiting.Reason == "CrashLoopBackOff" {
			return cs.Name, true
		}
	}
	return "", false
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	if m > 0 {
		return fmt.Sprintf("%dm%ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}
