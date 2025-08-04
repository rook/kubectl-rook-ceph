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
	"encoding/base64"
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
	PgMap  pgMap        `json:"pgmap"`
	Health healthStatus `json:"health"`
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

func Health(ctx context.Context, clientsets *k8sutil.Clientsets, operatorNamespace, clusterNamespace string) {
	logging.Info("Checking if at least three mon pods are running on different nodes")
	checkPodsOnNodes(ctx, clientsets.Kube, clusterNamespace, "app=rook-ceph-mon")

	fmt.Println()
	logging.Info("Checking mon quorum and ceph health details")
	checkMonQuorum(ctx, clientsets, operatorNamespace, clusterNamespace)

	fmt.Println()
	logging.Info("Checking if at least three osd pods are running on different nodes")
	checkPodsOnNodes(ctx, clientsets.Kube, clusterNamespace, "app=rook-ceph-osd")

	fmt.Println()
	CheckAllPodsStatus(ctx, clientsets.Kube, operatorNamespace, clusterNamespace)

	fmt.Println()
	logging.Info("Checking placement group status")
	checkPgStatus(ctx, clientsets, operatorNamespace, clusterNamespace)

	fmt.Println()
	logging.Info("Checking if at least one mgr pod is running")
	checkMgrPodsStatusAndCounts(ctx, clientsets.Kube, clusterNamespace)
}

func checkPodsOnNodes(ctx context.Context, k8sclientset kubernetes.Interface, clusterNamespace, label string) {
	var daemonType string
	if strings.Contains(label, "osd") {
		daemonType = "osd"
	} else if strings.Contains(label, "mon") {
		daemonType = "mon"
	}

	opts := metav1.ListOptions{LabelSelector: label}
	podList, err := k8sclientset.CoreV1().Pods(clusterNamespace).List(ctx, opts)
	if err != nil {
		logging.Error(fmt.Errorf("failed to list %s pods with label %s: %v", daemonType, opts.LabelSelector, err))
	}

	var nodeList = make(map[string]string)
	for i := range podList.Items {
		nodeName := podList.Items[i].Spec.NodeName
		if _, okay := nodeList[nodeName]; !okay {
			nodeList[nodeName] = podList.Items[i].Name
		}
	}

	if len(nodeList) < 3 {
		logging.Warning("At least three %s pods should running on different nodes\n", daemonType)
	}

	for i := range podList.Items {
		fmt.Printf("%s\t%s\t%s\t%s\n", podList.Items[i].Name, podList.Items[i].Status.Phase, podList.Items[i].Namespace, podList.Items[i].Spec.NodeName)
	}
}

func checkMonQuorum(ctx context.Context, clientsets *k8sutil.Clientsets, operatorNamespace, clusterNamespace string) {
	cephHealthDetails, _ := unMarshalCephStatus(ctx, clientsets, operatorNamespace, clusterNamespace)
	if cephHealthDetails == "HEALTH_OK" {
		logging.Info("%s", cephHealthDetails)
	} else if cephHealthDetails == "HEALTH_WARN" {
		logging.Warning("%s", cephHealthDetails)
	} else if cephHealthDetails == "HEALTH_ERR" {
		logging.Error(fmt.Errorf("%s", cephHealthDetails))
	}
}

func CheckAllPodsStatus(ctx context.Context, k8sclientset kubernetes.Interface, operatorNamespace, clusterNamespace string) {
	var podNotRunning, podRunning []v1.Pod
	podRunning, podNotRunning = getPodRunningStatus(ctx, k8sclientset, operatorNamespace)
	if operatorNamespace != clusterNamespace {
		clusterRunningPod, clusterNotRunningPod := getPodRunningStatus(ctx, k8sclientset, clusterNamespace)
		podRunning = append(podRunning, clusterRunningPod...)
		podNotRunning = append(podNotRunning, clusterNotRunningPod...)
	}

	logging.Info("Pods that are in 'Running' or `Succeeded` status")
	for i := range podRunning {
		fmt.Printf("%s \t %s \t %s\t %s\n", podRunning[i].Name, podRunning[i].Status.Phase, podRunning[i].Namespace, podRunning[i].Spec.NodeName)
	}

	fmt.Println()
	logging.Warning("Pods that are 'Not' in 'Running' status")
	for i := range podNotRunning {
		fmt.Printf("%s \t %s \t %s \t %s\n", podNotRunning[i].Name, podNotRunning[i].Status.Phase, podNotRunning[i].Namespace, podNotRunning[i].Spec.NodeName)
	}
}

func getPodRunningStatus(ctx context.Context, k8sclientset kubernetes.Interface, namespace string) ([]v1.Pod, []v1.Pod) {
	var podNotRunning, podRunning []v1.Pod
	podList, err := k8sclientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		logging.Error(fmt.Errorf("\nfailed to list pods in namespace %s: %v\n", namespace, err))
	}

	for i := range podList.Items {
		if podList.Items[i].Status.Phase != v1.PodRunning && podList.Items[i].Status.Phase != v1.PodSucceeded {
			podNotRunning = append(podNotRunning, podList.Items[i])
		} else {
			podRunning = append(podRunning, podList.Items[i])
		}
	}
	return podRunning, podNotRunning
}

func checkPgStatus(ctx context.Context, clientsets *k8sutil.Clientsets, operatorNamespace, clusterNamespace string) {
	_, pgStateEntryList := unMarshalCephStatus(ctx, clientsets, operatorNamespace, clusterNamespace)
	for _, pgStatus := range pgStateEntryList {
		if pgStatus.StateName == "active+clean" {
			logging.Info("\tPgState: %s, PgCount: %d", pgStatus.StateName, pgStatus.Count)
		} else if strings.Contains(pgStatus.StateName, "down") || strings.Contains(pgStatus.StateName, "incomplete") || strings.Contains(pgStatus.StateName, "snaptrim_error") {
			logging.Error(fmt.Errorf("\tPgState: %s, PgCount: %d", pgStatus.StateName, pgStatus.Count))
		} else {
			logging.Warning("\tPgState: %s, PgCount: %d", pgStatus.StateName, pgStatus.Count)
		}
	}
}

func checkMgrPodsStatusAndCounts(ctx context.Context, k8sclientset kubernetes.Interface, clusterNamespace string) {
	opts := metav1.ListOptions{LabelSelector: "app=rook-ceph-mgr"}
	podList, err := k8sclientset.CoreV1().Pods(clusterNamespace).List(ctx, opts)
	if err != nil {
		logging.Error(fmt.Errorf("\nfailed to list mgr pods with label %s: %v\n", opts.LabelSelector, err))
		return
	}

	if len(podList.Items) < 1 {
		logging.Warning("At least one mgr pod should be running")
	}

	for i := range podList.Items {
		fmt.Printf("%s\t%s\t%s\t%s\n", podList.Items[i].Name, podList.Items[i].Status.Phase, podList.Items[i].Namespace, podList.Items[i].Spec.NodeName)
	}
}

func unMarshalCephStatus(ctx context.Context, clientsets *k8sutil.Clientsets, operatorNamespace, clusterNamespace string) (string, []PgStateEntry) {
	cephStatusOut, err := exec.RunCommandInOperatorPod(ctx, clientsets, "ceph", []string{"-s", "--format", "json"}, operatorNamespace, clusterNamespace, true)
	if err != nil {
		logging.Fatal(err, "failed to get the status of ceph cluster")
	}

	ecodedText := base64.StdEncoding.EncodeToString([]byte(cephStatusOut))
	decodeCephStatus, err := base64.StdEncoding.DecodeString(ecodedText)
	if err != nil {
		logging.Fatal(err)
	}
	var cephStatus *cephStatus

	err = json.Unmarshal(decodeCephStatus, &cephStatus)
	if err != nil {
		logging.Fatal(err)
	}
	return cephStatus.Health.Status, cephStatus.PgMap.PgsByState
}
