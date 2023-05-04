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
	ctx "context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/rook/kubectl-rook-ceph/pkg/exec"
	"github.com/rook/kubectl-rook-ceph/pkg/k8sutil"
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

func Health(context *k8sutil.Context, operatorNamespace, clusterNamespace string) {

	fmt.Println("Info: Checking if at least three mon pods are running on different nodes")
	checkPodsOnNodes(context, clusterNamespace, "app=rook-ceph-mon")

	fmt.Println()
	fmt.Println("Info: Checking mon quorum and ceph health details")
	checkMonQuorum(context, operatorNamespace, clusterNamespace)

	fmt.Println()
	fmt.Println("Info: Checking if at least three osd pods are running on different nodes")
	checkPodsOnNodes(context, clusterNamespace, "app=rook-ceph-osd")

	fmt.Println()
	CheckAllPodsStatus(context, operatorNamespace, clusterNamespace)

	fmt.Println("Info: Checking placement group status")
	checkPgStatus(context, operatorNamespace, clusterNamespace)

	fmt.Println()
	fmt.Println("Info: Checking if at least one mgr pod is running")
	checkMgrPodsStatusAndCounts(context, clusterNamespace)
}

func checkPodsOnNodes(context *k8sutil.Context, clusterNamespace, label string) {
	var daemonType string
	if strings.Contains(label, "osd") {
		daemonType = "osd"
	} else if strings.Contains(label, "mon") {
		daemonType = "mon"
	}

	opts := metav1.ListOptions{LabelSelector: label}
	podList, err := context.Clientset.CoreV1().Pods(clusterNamespace).List(ctx.TODO(), opts)
	if err != nil {
		fmt.Printf("\nfailed to list %s pods with label %s: %v\n", daemonType, opts.LabelSelector, err)
		return
	}

	var nodeList = make(map[string]string)
	for i := range podList.Items {
		nodeName := podList.Items[i].Spec.NodeName
		if _, okay := nodeList[nodeName]; !okay {
			nodeList[nodeName] = podList.Items[i].Name
		}
	}

	if len(nodeList) < 3 {
		fmt.Printf("\nWarning: At least three %s pods should running on different nodes\n", daemonType)
	}

	for i := range podList.Items {
		fmt.Println(podList.Items[i].Name, "\t", podList.Items[i].Status.Phase, "\t", podList.Items[i].Namespace, "\t", podList.Items[i].Spec.NodeName)
	}
}

func checkMonQuorum(context *k8sutil.Context, operatorNamespace, clusterNamespace string) {
	cephHealthStatus, _ := unMarshalCephStatus(context, operatorNamespace, clusterNamespace)
	if cephHealthStatus == "HEALTH_OK" {
		fmt.Println("Info:", cephHealthStatus)
	} else if cephHealthStatus == "HEALTH_WARN" {
		fmt.Println("Warning:", cephHealthStatus)
	} else if cephHealthStatus == "HEALTH_ERR" {
		fmt.Println("Error: ", cephHealthStatus)
	}
}

func CheckAllPodsStatus(context *k8sutil.Context, operatorNamespace, clusterNamespace string) {
	var podNotRunning, podRunning []v1.Pod
	podRunning, podNotRunning = getPodRunningStatus(context, operatorNamespace)
	if operatorNamespace != clusterNamespace {
		clusterRunningPod, clusterNotRunningPod := getPodRunningStatus(context, clusterNamespace)
		podRunning = append(podRunning, clusterRunningPod...)
		podNotRunning = append(podNotRunning, clusterNotRunningPod...)
	}

	if podRunning != nil {
		fmt.Println("Info: Pods that are in 'Running' status")
		for i := range podRunning {
			fmt.Println(podRunning[i].Name, "\t", podRunning[i].Status.Phase, "\t", podRunning[i].Namespace, "\t", podRunning[i].Spec.NodeName)
		}
	}

	if podNotRunning != nil {
		fmt.Println("\nWarning: Pods that are 'Not' in 'Running' status")
		for i := range podNotRunning {
			fmt.Println(podNotRunning[i].Name, "\t", podNotRunning[i].Status.Phase, "\t", podNotRunning[i].Namespace, "\t", podNotRunning[i].Spec.NodeName)
		}
	}
}

func getPodRunningStatus(context *k8sutil.Context, namespace string) ([]v1.Pod, []v1.Pod) {
	var podNotRunning, podRunning []v1.Pod
	podList, err := context.Clientset.CoreV1().Pods(namespace).List(ctx.TODO(), metav1.ListOptions{})
	if err != nil {
		fmt.Printf("\nfailed to list pods in namespace %s: %v\n", namespace, err)
		return []v1.Pod{}, []v1.Pod{}
	}

	for i := range podList.Items {
		if podList.Items[i].Status.Phase != v1.PodRunning {
			podNotRunning = append(podNotRunning, podList.Items[i])
		} else {
			podRunning = append(podRunning, podList.Items[i])
		}
	}
	return podRunning, podNotRunning
}

func checkPgStatus(context *k8sutil.Context, operatorNamespace, clusterNamespace string) {
	_, pgStateEntryList := unMarshalCephStatus(context, operatorNamespace, clusterNamespace)
	for _, pgStatus := range pgStateEntryList {
		if pgStatus.StateName == "active+clean" {
			fmt.Println("Info:\n", "\tPgState: ", pgStatus.StateName+",", "PgCount: ", pgStatus.Count)
		} else if strings.Contains(pgStatus.StateName, "down") || strings.Contains(pgStatus.StateName, "incomplete") || strings.Contains(pgStatus.StateName, "snaptrim_error") {
			fmt.Println("Warning:\n", "\tPgState: ", pgStatus.StateName+",", "PgCount: ", pgStatus.Count)
		} else {
			fmt.Println("Error:\n", "\tPgState: ", pgStatus.StateName+",", "PgCount: ", pgStatus.Count)
		}
	}
}

func checkMgrPodsStatusAndCounts(context *k8sutil.Context, clusterNamespace string) {
	opts := metav1.ListOptions{LabelSelector: "app=rook-ceph-mgr"}
	podList, err := context.Clientset.CoreV1().Pods(clusterNamespace).List(ctx.TODO(), opts)
	if err != nil {
		fmt.Printf("\nfailed to list mgr pods with label %s: %v\n", opts.LabelSelector, err)
		return
	}

	if len(podList.Items) < 1 {
		fmt.Println("At least one mgr pod should be running")
	}
	for i := range podList.Items {
		fmt.Println(podList.Items[i].Name, "\t", podList.Items[i].Status.Phase, "\t", podList.Items[i].Namespace, "\t", podList.Items[i].Spec.NodeName)
	}
}

func unMarshalCephStatus(context *k8sutil.Context, operatorNamespace, clusterNamespace string) (string, []PgStateEntry) {
	cephStatusOut := exec.RunCommandInOperatorPod(context, "ceph", []string{"-s", "--format", "json"}, operatorNamespace, clusterNamespace, false)

	ecodedText := base64.StdEncoding.EncodeToString([]byte(cephStatusOut))
	decodeCephStatus, err := base64.StdEncoding.DecodeString(ecodedText)
	if err != nil {
		log.Fatal(err)
	}
	var cephStatus *cephStatus

	err = json.Unmarshal(decodeCephStatus, &cephStatus)
	if err != nil {
		log.Fatal(err)
	}
	return cephStatus.Health.Status, cephStatus.PgMap.PgsByState
}
