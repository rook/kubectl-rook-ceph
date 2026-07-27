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
	"strings"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

var pressureConditions = []v1.NodeConditionType{
	v1.NodeMemoryPressure,
	v1.NodeDiskPressure,
	v1.NodePIDPressure,
}

func checkNodeResourcePressure(ctx context.Context, k8sclientset kubernetes.Interface, nodeSelector string) CheckResult {
	result := CheckResult{
		Name:     CheckNodeResourcePressure,
		Category: CategoryK8sResources,
	}

	nodes, err := k8sclientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{
		LabelSelector: nodeSelector,
	})
	if err != nil {
		result.Status = StatusError
		result.Message = fmt.Sprintf("Failed to list nodes: %v", err)
		return result
	}

	if len(nodes.Items) == 0 {
		result.Status = StatusOK
		result.Message = "No nodes found"
		return result
	}

	pressuredNodes := 0
	notReadyNodes := 0

	for i := range nodes.Items {
		node := &nodes.Items[i]
		pressures := getNodePressures(node)
		ready := isNodeReady(node)

		nodeStatus := "Ready"
		if !ready {
			nodeStatus = "NotReady"
			notReadyNodes++
		}

		condSummary := nodeConditionSummary(node)
		result.Items = append(result.Items, CheckItem{
			Name:    node.Name,
			Status:  nodeStatus,
			Details: condSummary,
		})

		if len(pressures) > 0 || !ready {
			var parts []string
			if !ready {
				parts = append(parts, "NotReady")
			}
			parts = append(parts, pressures...)
			result.Details = append(result.Details, fmt.Sprintf("[WARN] %s: %s", node.Name, strings.Join(parts, ", ")))
			if len(pressures) > 0 {
				pressuredNodes++
			}
		}
	}

	totalNodes := len(nodes.Items)

	if pressuredNodes == 0 && notReadyNodes == 0 {
		result.Status = StatusOK
		result.Message = fmt.Sprintf("All %d nodes are healthy", totalNodes)
		result.Details = append(result.Details, fmt.Sprintf("[INFO] %d nodes checked, no resource pressure detected", totalNodes))
		return result
	}

	result.Status = StatusWarning
	var msgs []string
	if pressuredNodes > 0 {
		msgs = append(msgs, fmt.Sprintf("%d node(s) under resource pressure", pressuredNodes))
	}
	if notReadyNodes > 0 {
		msgs = append(msgs, fmt.Sprintf("%d node(s) not ready", notReadyNodes))
		result.Status = StatusCritical
	}
	result.Message = strings.Join(msgs, ", ")

	return result
}

func getNodePressures(node *v1.Node) []string {
	var pressures []string
	for _, condType := range pressureConditions {
		for _, cond := range node.Status.Conditions {
			if cond.Type == condType && cond.Status == v1.ConditionTrue {
				pressures = append(pressures, string(condType))
			}
		}
	}
	return pressures
}

func isNodeReady(node *v1.Node) bool {
	for _, cond := range node.Status.Conditions {
		if cond.Type == v1.NodeReady {
			return cond.Status == v1.ConditionTrue
		}
	}
	return false
}

func nodeConditionSummary(node *v1.Node) string {
	conditionTypes := []v1.NodeConditionType{
		v1.NodeReady,
		v1.NodeMemoryPressure,
		v1.NodeDiskPressure,
		v1.NodePIDPressure,
	}

	var parts []string
	for _, ct := range conditionTypes {
		for _, cond := range node.Status.Conditions {
			if cond.Type == ct {
				parts = append(parts, fmt.Sprintf("%s=%s", cond.Type, cond.Status))
			}
		}
	}
	return strings.Join(parts, ", ")
}
