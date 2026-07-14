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

package health

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rook/kubectl-rook-ceph/pkg/exec"
	"github.com/rook/kubectl-rook-ceph/pkg/k8sutil"
	"github.com/rook/kubectl-rook-ceph/pkg/logging"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type cephStatus struct {
	PgMap       pgMap        `json:"pgmap"`
	Health      healthStatus `json:"health"`
	QuorumNames []string     `json:"quorum_names"`
	MonMap      monMap       `json:"monmap"`
}

type monMap struct {
	NumMons int        `json:"num_mons"`
	Mons    []monEntry `json:"mons"`
}

type monEntry struct {
	Name string `json:"name"`
	Rank int    `json:"rank"`
}

type healthStatus struct {
	Status string `json:"status"`
}

type pgMap struct {
	PgsByState []PgStateEntry `json:"pgs_by_state"`
}

type PgStateEntry struct {
	StateName string `json:"state_name"`
	Count     int    `json:"count"`
}

func Health(ctx context.Context, clientsets *k8sutil.Clientsets, operatorNamespace, clusterNamespace string, verbose bool) {
	var results []CheckResult

	logging.Plain("Checking Ceph status...")
	cephStatus, cephStatusErr := unMarshalCephStatus(ctx, clientsets, operatorNamespace, clusterNamespace)

	checks := []struct {
		name string
		run  func() CheckResult
	}{
		{CheckMonDistribution, func() CheckResult {
			return checkMonDistribution(ctx, clientsets.Kube, clusterNamespace, cephStatus, cephStatusErr)
		}},
		{CheckCephClusterHealth, func() CheckResult {
			return checkCephClusterHealth(cephStatus, cephStatusErr)
		}},
		{CheckOSDDistribution, func() CheckResult {
			return checkOSDDistribution(ctx, clientsets.Kube, clusterNamespace)
		}},
		{CheckAllPodsStatus, func() CheckResult {
			return checkAllPodsStatus(ctx, clientsets.Kube, operatorNamespace, clusterNamespace)
		}},
		{CheckPGStatus, func() CheckResult {
			return checkPGStatus(cephStatus, cephStatusErr)
		}},
		{CheckMGRStatus, func() CheckResult {
			return checkMGRStatus(ctx, clientsets.Kube, clusterNamespace)
		}},
	}

	for _, c := range checks {
		logging.Plain("Checking %s...", c.name)
		results = append(results, c.run())
	}

	printReport(clusterNamespace, results, verbose)
}

func checkMonDistribution(ctx context.Context, k8sclientset kubernetes.Interface, clusterNamespace string, status cephStatus, statusErr error) CheckResult {
	result := checkPodsOnNodes(ctx, k8sclientset, clusterNamespace, "app=rook-ceph-mon", CheckMonDistribution, "mon")

	if statusErr != nil {
		result.Details = append(result.Details, fmt.Sprintf("Could not check mon quorum: %v", statusErr))
		result.Status = StatusError
		return result
	}

	quorumStatus, quorumDetails := evaluateMonQuorum(status.QuorumNames, status.MonMap.Mons, status.MonMap.NumMons)
	result.Details = append(result.Details, quorumDetails...)
	result.Status = worseStatus(result.Status, quorumStatus)

	return result
}

func checkOSDDistribution(ctx context.Context, k8sclientset kubernetes.Interface, clusterNamespace string) CheckResult {
	return checkPodsOnNodes(ctx, k8sclientset, clusterNamespace, "app=rook-ceph-osd", CheckOSDDistribution, "osd")
}

func checkPodsOnNodes(ctx context.Context, k8sclientset kubernetes.Interface, clusterNamespace, label, checkName, daemonType string) CheckResult {
	result := CheckResult{
		Name:     checkName,
		Category: CategoryK8sResources,
	}

	opts := metav1.ListOptions{LabelSelector: label}
	podList, err := k8sclientset.CoreV1().Pods(clusterNamespace).List(ctx, opts)
	if err != nil {
		result.Status = StatusError
		result.Message = fmt.Sprintf("Failed to list %s pods: %v", daemonType, err)
		return result
	}

	uniqueNodeSet := make(map[string]bool)
	runningCount := 0
	notRunningCount := 0
	for i := range podList.Items {
		pod := &podList.Items[i]
		result.Items = append(result.Items, CheckItem{
			Name:   pod.Name,
			Status: string(pod.Status.Phase),
			Node:   pod.Spec.NodeName,
		})
		if pod.Status.Phase == v1.PodRunning {
			runningCount++
			if pod.Spec.NodeName != "" {
				uniqueNodeSet[pod.Spec.NodeName] = true
			}
		} else {
			notRunningCount++
		}
	}

	uniqueNodes := len(uniqueNodeSet)

	result.Status = StatusOK
	result.Message = fmt.Sprintf("%d %s pods running on %d different nodes", runningCount, daemonType, uniqueNodes)

	if notRunningCount > 0 || uniqueNodes < 3 {
		result.Status = StatusWarning
		msg := fmt.Sprintf("%d %s pods running on %d different nodes", runningCount, daemonType, uniqueNodes)
		if notRunningCount > 0 {
			msg += fmt.Sprintf(", %d pods not running", notRunningCount)
		}
		if uniqueNodes < 3 {
			msg += " (at least 3 recommended)"
		}
		result.Message = msg
	}

	return result
}

func checkCephClusterHealth(status cephStatus, statusErr error) CheckResult {
	result := CheckResult{
		Name:     CheckCephClusterHealth,
		Category: CategoryStorage,
	}

	if statusErr != nil {
		result.Status = StatusError
		result.Message = statusErr.Error()
		return result
	}

	switch status.Health.Status {
	case "HEALTH_OK":
		result.Status = StatusOK
		result.Message = status.Health.Status
	case "HEALTH_WARN":
		result.Status = StatusWarning
		result.Message = status.Health.Status
	case "HEALTH_ERR":
		result.Status = StatusCritical
		result.Message = status.Health.Status
	default:
		result.Status = StatusError
		result.Message = fmt.Sprintf("Unexpected health status: %s", status.Health.Status)
	}

	return result
}

func checkAllPodsStatus(ctx context.Context, k8sclientset kubernetes.Interface, operatorNamespace, clusterNamespace string) CheckResult {
	result := CheckResult{
		Name:     CheckAllPodsStatus,
		Category: CategoryK8sResources,
	}

	podRunning, podNotRunning, err := getPodRunningStatus(ctx, k8sclientset, operatorNamespace)
	if err != nil {
		result.Status = StatusError
		result.Message = err.Error()
		return result
	}
	if operatorNamespace != clusterNamespace {
		clusterRunning, clusterNotRunning, err := getPodRunningStatus(ctx, k8sclientset, clusterNamespace)
		if err != nil {
			result.Status = StatusError
			result.Message = err.Error()
			return result
		}
		podRunning = append(podRunning, clusterRunning...)
		podNotRunning = append(podNotRunning, clusterNotRunning...)
	}

	runningCount := len(podRunning)
	notRunningCount := len(podNotRunning)

	for i := range podNotRunning {
		pod := &podNotRunning[i]
		result.Items = append(result.Items, CheckItem{
			Name:      pod.Name,
			Namespace: pod.Namespace,
			Status:    string(pod.Status.Phase),
			Node:      pod.Spec.NodeName,
		})
	}

	result.Status = StatusOK
	result.Message = fmt.Sprintf("All %d pods are Running/Succeeded", runningCount)

	if notRunningCount > 0 {
		result.Status = StatusWarning
		result.Message = fmt.Sprintf("%d pods Running/Succeeded, %d pods not running", runningCount, notRunningCount)
	}

	result.Details = append(result.Details, fmt.Sprintf("%d Running/Succeeded across checked namespaces", runningCount))

	return result
}

func getPodRunningStatus(ctx context.Context, k8sclientset kubernetes.Interface, namespace string) ([]v1.Pod, []v1.Pod, error) {
	var podNotRunning, podRunning []v1.Pod
	podList, err := k8sclientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list pods in namespace %s: %v", namespace, err)
	}

	for i := range podList.Items {
		if podList.Items[i].Status.Phase != v1.PodRunning && podList.Items[i].Status.Phase != v1.PodSucceeded {
			podNotRunning = append(podNotRunning, podList.Items[i])
		} else {
			podRunning = append(podRunning, podList.Items[i])
		}
	}
	return podRunning, podNotRunning, nil
}

func checkPGStatus(status cephStatus, statusErr error) CheckResult {
	result := CheckResult{
		Name:     CheckPGStatus,
		Category: CategoryStorage,
	}

	if statusErr != nil {
		result.Status = StatusError
		result.Message = statusErr.Error()
		return result
	}

	overallStatus := StatusOK
	for _, pg := range status.PgMap.PgsByState {
		detail := fmt.Sprintf("PgState: %s, PgCount: %d", pg.StateName, pg.Count)
		result.Details = append(result.Details, detail)

		if isHealthyPGState(pg.StateName) {
			continue
		}
		if strings.Contains(pg.StateName, "down") || strings.Contains(pg.StateName, "incomplete") || strings.Contains(pg.StateName, "snaptrim_error") {
			overallStatus = StatusCritical
		} else if overallStatus != StatusCritical {
			overallStatus = StatusWarning
		}
	}

	result.Status = overallStatus
	switch overallStatus {
	case StatusOK:
		total := 0
		for _, pg := range status.PgMap.PgsByState {
			total += pg.Count
		}
		result.Message = fmt.Sprintf("%d PGs in healthy state", total)
	case StatusWarning:
		result.Message = "Some PGs are not in healthy state"
	case StatusCritical:
		result.Message = "PGs in critical state detected"
	}

	return result
}

func checkMGRStatus(ctx context.Context, k8sclientset kubernetes.Interface, clusterNamespace string) CheckResult {
	result := CheckResult{
		Name:     CheckMGRStatus,
		Category: CategoryK8sResources,
	}

	opts := metav1.ListOptions{LabelSelector: "app=rook-ceph-mgr"}
	podList, err := k8sclientset.CoreV1().Pods(clusterNamespace).List(ctx, opts)
	if err != nil {
		result.Status = StatusError
		result.Message = fmt.Sprintf("Failed to list mgr pods: %v", err)
		return result
	}

	runningCount := 0
	for i := range podList.Items {
		pod := &podList.Items[i]
		result.Items = append(result.Items, CheckItem{
			Name:   pod.Name,
			Status: string(pod.Status.Phase),
			Node:   pod.Spec.NodeName,
		})
		if pod.Status.Phase == v1.PodRunning {
			runningCount++
		}
	}

	result.Status = StatusOK
	result.Message = fmt.Sprintf("%d mgr pod(s) running", runningCount)

	if runningCount < 1 {
		result.Status = StatusWarning
		result.Message = "No mgr pods running (at least 1 required)"
	}

	return result
}

func worseStatus(a, b CheckStatus) CheckStatus {
	if a > b {
		return a
	}
	return b
}

func evaluateMonQuorum(quorumNames []string, mons []monEntry, totalMons int) (CheckStatus, []string) {
	if totalMons == 0 {
		return StatusOK, []string{"Mon quorum data not available in ceph status"}
	}

	quorumSet := make(map[string]bool, len(quorumNames))
	for _, name := range quorumNames {
		quorumSet[name] = true
	}

	inQuorum := len(quorumNames)
	var notInQuorum []string
	for _, mon := range mons {
		if !quorumSet[mon.Name] {
			notInQuorum = append(notInQuorum, mon.Name)
		}
	}

	var details []string
	details = append(details, fmt.Sprintf("%d/%d mons in quorum", inQuorum, totalMons))

	if len(notInQuorum) == 0 {
		return StatusOK, details
	}

	for _, name := range notInQuorum {
		details = append(details, fmt.Sprintf("Mon %s not in quorum", name))
	}

	majority := totalMons/2 + 1
	if inQuorum >= majority {
		return StatusWarning, details
	}
	return StatusCritical, details
}

var healthyPGStates = map[string]bool{
	"active+clean":                true,
	"active+clean+snaptrim":       true,
	"active+clean+snaptrim_wait":  true,
	"active+clean+scrubbing":      true,
	"active+clean+scrubbing+deep": true,
}

func isHealthyPGState(state string) bool {
	return healthyPGStates[state]
}

func unMarshalCephStatus(ctx context.Context, clientsets *k8sutil.Clientsets, operatorNamespace, clusterNamespace string) (cephStatus, error) {
	cephStatusOut, err := exec.RunCommandInOperatorPod(ctx, clientsets, "ceph", []string{"-s", "--format", "json"}, operatorNamespace, clusterNamespace, true)
	if err != nil {
		return cephStatus{}, fmt.Errorf("failed to get ceph status: %v", err)
	}

	var status cephStatus
	err = json.Unmarshal([]byte(cephStatusOut), &status)
	if err != nil {
		return cephStatus{}, fmt.Errorf("failed to unmarshal ceph status: %v", err)
	}
	return status, nil
}
