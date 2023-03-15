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

package mons

import (
	ctx "context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/rook/kubectl-rook-ceph/pkg/debug"
	"github.com/rook/kubectl-rook-ceph/pkg/exec"
	"github.com/rook/kubectl-rook-ceph/pkg/k8sutil"

	log "github.com/sirupsen/logrus"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func RestoreQuorum(context *k8sutil.Context, operatorNamespace, clusterNamespace, goodMon string) {
	err := restoreQuorum(context, operatorNamespace, clusterNamespace, goodMon)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func restoreQuorum(context *k8sutil.Context, operatorNamespace, clusterNamespace, goodMon string) error {
	monCm, err := context.Clientset.CoreV1().ConfigMaps(clusterNamespace).Get(ctx.TODO(), MonConfigMap, v1.GetOptions{})
	if err != nil {
		log.Fatalf("failed to get mon configmap %s %v", MonConfigMap, err)
	}

	monData := monCm.Data["data"]
	monEndpoints := strings.Split(monData, ",")

	badMons, goodMonPublicIp, goodMonPort, err := getMonDetails(goodMon, monEndpoints)
	if err != nil {
		log.Fatal(err)
	}

	if goodMonPublicIp == "" {
		return fmt.Errorf("error: good mon %s not found", goodMon)
	}

	fsidSecret, err := context.Clientset.CoreV1().Secrets(clusterNamespace).Get(ctx.TODO(), "rook-ceph-mon", v1.GetOptions{})
	if err != nil {
		log.Fatalf("failed to get mon configmap %s %v", MonConfigMap, err)
	}

	cephFsid := string(fsidSecret.Data["fsid"])
	if cephFsid == "" {
		return fmt.Errorf("ceph cluster fsid not found")
	}

	fmt.Printf("printing fsid secret %s\n", cephFsid)
	fmt.Println("Check for the running toolbox")

	_, err = debug.GetDeployment(context, clusterNamespace, "rook-ceph-tools")
	if err != nil {
		return fmt.Errorf("failed to deployment rook-ceph-tools. %v", err)
	}

	toolBox, err := k8sutil.WaitForPodToRun(context, clusterNamespace, "app=rook-ceph-tools")
	if err != nil || toolBox.Name == "" {
		return fmt.Errorf("failed to get the running toolbox")
	}

	fmt.Printf("Restoring mon quorum to mon %s %s\n", goodMon, goodMonPublicIp)
	fmt.Printf("The mons to discard are: %s\n", badMons)
	fmt.Printf("The cluster fsid is %s\n", cephFsid)

	var answer, output string
	fmt.Printf("Are you sure you want to restore the quorum to mon %s? If so, enter 'yes-really-restore\n", goodMon)
	fmt.Scanf("%s", &answer)
	output, err = promptToContinueOrCancel(answer)
	if err != nil {
		return fmt.Errorf(" restoring the mon quorum to mon %s cancelled", goodMon)
	}
	fmt.Println(output)

	fmt.Println("Waiting for operator pod to stop")
	err = debug.SetDeploymentScale(context, operatorNamespace, "rook-ceph-operator", 0)
	if err != nil {
		return fmt.Errorf("failed to stop deployment rook-ceph-operator. %v", err)
	}
	fmt.Println("rook-ceph-operator deployment scaled down")

	fmt.Println("Waiting for bad mon pod to stop")
	for _, badMon := range badMons {
		err = debug.SetDeploymentScale(context, clusterNamespace, fmt.Sprintf("rook-ceph-mon-%s", badMon), 0)
		if err != nil {
			return fmt.Errorf("deployment %s still exist. %v", fmt.Sprintf("rook-ceph-mon-%s", badMon), err)
		}
		fmt.Printf("deployment.apps/%s scaled\n", fmt.Sprintf("rook-ceph-mon-%s", badMon))
	}

	debug.StartDebug(context, clusterNamespace, fmt.Sprintf("rook-ceph-mon-%s", goodMon), "")

	debugDeploymentSpec, err := debug.GetDeployment(context, clusterNamespace, fmt.Sprintf("rook-ceph-mon-%s-debug", goodMon))
	if err != nil {
		return fmt.Errorf("failed to deployment rook-ceph-mon-%s-debug", goodMon)
	}

	labelSelector := fmt.Sprintf("ceph_daemon_type=%s,ceph_daemon_id=%s", debugDeploymentSpec.Spec.Template.Labels["ceph_daemon_type"], debugDeploymentSpec.Spec.Template.Labels["ceph_daemon_id"])
	_, err = k8sutil.WaitForPodToRun(context, clusterNamespace, labelSelector)
	if err != nil {
		return fmt.Errorf("failed to start deployment %s", fmt.Sprintf("rook-ceph-mon-%s-debug", goodMon))
	}

	updateMonMap(context, clusterNamespace, labelSelector, cephFsid, goodMon, goodMonPublicIp, badMons)

	fmt.Println("Restoring the mons in the rook-ceph-mon-endpoints configmap to the good mon")
	monCm.Data["data"] = fmt.Sprintf("%s=%s:%s", goodMon, goodMonPublicIp, goodMonPort)

	monCm, err = context.Clientset.CoreV1().ConfigMaps(clusterNamespace).Update(ctx.TODO(), monCm, v1.UpdateOptions{})
	if err != nil {
		log.Fatalf("failed to update mon configmap %s %v", MonConfigMap, err)
	}

	fmt.Printf("Stopping the debug pod for mon %s.\n", goodMon)
	debug.StopDebug(context, clusterNamespace, fmt.Sprintf("rook-ceph-mon-%s", goodMon))

	fmt.Println("Check that the restored mon is responding")
	err = waitForMonStatusResponse(context, clusterNamespace)
	if err != nil {
		return err
	}

	err = removeBadMonsResources(context, clusterNamespace, badMons)
	if err != nil {
		return err
	}

	fmt.Printf("Mon quorum was successfully restored to mon %s\n", goodMon)
	fmt.Println("Only a single mon is currently running")

	output, err = promptToContinueOrCancel(answer)
	if err != nil {
		return fmt.Errorf(" restoring the mon quorum to mon %s cancelled", goodMon)
	}
	fmt.Println(output)

	err = debug.SetDeploymentScale(context, clusterNamespace, "rook-ceph-operator", 1)
	if err != nil {
		return fmt.Errorf("failed to start deployment rook-ceph-operator. %v", err)
	}

	return nil
}

func updateMonMap(context *k8sutil.Context, clusterNamespace, labelSelector, cephFsid, goodMon, goodMonPublicIp string, badMons []string) {
	fmt.Println("Started debug pod, restoring the mon quorum in the debug pod")

	monmapPath := "/tmp/monmap"

	monMapArgs := []string{
		fmt.Sprintf("--fsid=%s", cephFsid),
		"--keyring=/etc/ceph/keyring-store/keyring",
		"--log-to-stderr=true",
		"--err-to-stderr=true",
		"--mon-cluster-log-to-stderr=true",
		"--log-stderr-prefix=debug",
		"--default-log-to-file=false",
		"--default-mon-cluster-log-to-file=false",
		"--mon-host=$(ROOK_CEPH_MON_HOST)",
		"--mon-initial-members=$(ROOK_CEPH_MON_INITIAL_MEMBERS)",
		fmt.Sprintf("--id=%s", goodMon),
		"--foreground",
		fmt.Sprintf("--public-addr=%s", goodMonPublicIp),
		fmt.Sprintf("--setuser-match-path=/var/lib/ceph/mon/ceph-%s/store.db", goodMon),
		"--public-bind-addr=",
	}

	extractMonMap := []string{fmt.Sprintf("--extract-monmap=%s", monmapPath)}
	extractMonMapArgs := append(monMapArgs, extractMonMap...)

	fmt.Println("Extracting the monmap")
	fmt.Println(exec.RunCommandInLabeledPod(context, labelSelector, "mon", "ceph-mon", extractMonMapArgs, clusterNamespace, true))

	fmt.Println("Printing monmap")
	fmt.Println(exec.RunCommandInLabeledPod(context, labelSelector, "mon", "monmaptool", []string{"--print", monmapPath}, clusterNamespace, true))

	// remove all the mons except the good one
	for _, badMonId := range badMons {
		fmt.Printf("Removing mon %s.\n", badMonId)
		fmt.Println(exec.RunCommandInLabeledPod(context, labelSelector, "mon", "monmaptool", []string{monmapPath, "--rm", badMonId}, clusterNamespace, true))
	}

	injectMonMap := []string{fmt.Sprintf("--inject-monmap=%s", monmapPath)}
	injectMonMapArgs := append(monMapArgs, injectMonMap...)

	fmt.Println("Injecting the monmap")
	fmt.Println(exec.RunCommandInLabeledPod(context, labelSelector, "mon", "ceph-mon", injectMonMapArgs, clusterNamespace, true))

	fmt.Println("Finished updating the monmap!")

	fmt.Println("Printing final monmap")
	fmt.Println(exec.RunCommandInLabeledPod(context, labelSelector, "mon", "monmaptool", []string{"--print", monmapPath}, clusterNamespace, true))
}

func removeBadMonsResources(context *k8sutil.Context, clusterNamespace string, badMons []string) error {
	fmt.Printf("Purging the bad mons %v\n", badMons)

	for _, badMon := range badMons {
		fmt.Printf("purging bad mon: %s\n", badMon)
		err := context.Clientset.AppsV1().Deployments(clusterNamespace).Delete(ctx.TODO(), fmt.Sprintf("rook-ceph-mon-%s", badMon), v1.DeleteOptions{})
		if err != nil {
			return fmt.Errorf("failed to delete deployment %s", fmt.Sprintf("rook-ceph-mon-%s", badMon))
		}
		err = context.Clientset.CoreV1().Services(clusterNamespace).Delete(ctx.TODO(), fmt.Sprintf("rook-ceph-mon-%s", badMon), v1.DeleteOptions{})
		if err != nil && !kerrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete service %s", fmt.Sprintf("rook-ceph-mon-%s", badMon))
		}

		err = context.Clientset.CoreV1().PersistentVolumeClaims(clusterNamespace).Delete(ctx.TODO(), fmt.Sprintf("rook-ceph-mon-%s", badMon), v1.DeleteOptions{})
		if err != nil && !kerrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete pvc %s", fmt.Sprintf("rook-ceph-mon-%s", badMon))
		}
	}
	return nil
}

func waitForMonStatusResponse(context *k8sutil.Context, clusterNamespace string) error {
	maxRetries := 20

	for i := 0; i < maxRetries; i++ {
		output := exec.RunCommandInToolboxPod(context, "ceph", []string{"status"}, clusterNamespace, false)
		if strings.Contains(output, "HEALTH_WARN") || strings.Contains(output, "HEALTH_OK") || strings.Contains(output, "HEALTH_ERROR") {
			fmt.Printf("finished waiting for ceph status %s\n", output)
			break
		}
		if i == maxRetries-1 {
			return fmt.Errorf("timed out waiting for mon quorum to respond")
		}
		fmt.Printf("%d: waiting for ceph status to confirm single mon quorum. \n", i+1)
		fmt.Printf("current ceph status output %s\n", output)
		fmt.Println("sleeping for 5 seconds")
		time.Sleep(5 * time.Second)
	}

	return nil
}

func getMonDetails(goodMon string, monEndpoints []string) ([]string, string, string, error) {
	var goodMonPublicIp, goodMonPort string
	var badMons []string

	for _, m := range monEndpoints {
		monName, monEndpoint, ok := strings.Cut(m, "=")
		if !ok {
			return []string{}, "", "", fmt.Errorf("failed to fetch mon endpoint")
		} else if monName == goodMon {
			goodMonPublicIp, goodMonPort, ok = strings.Cut(monEndpoint, ":")
			if !ok {
				return []string{}, "", "", fmt.Errorf("failed to get good mon endpoint and port")
			}
		} else {
			badMons = append(badMons, monName)
		}
		fmt.Printf("mon=%s, endpoints=%s\n", monName, monEndpoint)
	}
	return badMons, goodMonPublicIp, goodMonPort, nil
}

func promptToContinueOrCancel(answer string) (string, error) {
	var ROOK_PLUGIN_SKIP_PROMPTS string
	_, ok := os.LookupEnv(ROOK_PLUGIN_SKIP_PROMPTS)
	if ok {
		if answer == "yes-really-restore" {
			return "proceeding", nil
		} else if answer == "" {
			return "continuing", nil
		} else {
			return "", fmt.Errorf("canncelled")
		}
	} else {
		return "skipped prompt since ROOK_PLUGIN_SKIP_PROMPTS=true", nil
	}
}
