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
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/rook/kubectl-rook-ceph/pkg/debug"
	"github.com/rook/kubectl-rook-ceph/pkg/exec"
	"github.com/rook/kubectl-rook-ceph/pkg/k8sutil"
	"github.com/rook/kubectl-rook-ceph/pkg/logging"

	kerrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func RestoreQuorum(ctx context.Context, clientsets *k8sutil.Clientsets, operatorNamespace, clusterNamespace, goodMon string) {
	err := validateMonIsUp(ctx, clientsets, clusterNamespace, goodMon)
	if err != nil {
		logging.Fatal(err)
	}

	err = restoreQuorum(ctx, clientsets, operatorNamespace, clusterNamespace, goodMon)
	if err != nil {
		logging.Fatal(err)
	}
}

func restoreQuorum(ctx context.Context, clientsets *k8sutil.Clientsets, operatorNamespace, clusterNamespace, goodMon string) error {
	monCm, err := clientsets.Kube.CoreV1().ConfigMaps(clusterNamespace).Get(ctx, MonConfigMap, v1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get mon configmap %s %v", MonConfigMap, err)
	}

	monData := monCm.Data["data"]
	monEndpoints := strings.Split(monData, ",")

	badMons, goodMonPublicIp, goodMonPort, err := getMonDetails(goodMon, monEndpoints)
	if err != nil {
		return err
	}

	if goodMonPublicIp == "" {
		return fmt.Errorf("good mon %s not found", goodMon)
	}

	fsidSecret, err := clientsets.Kube.CoreV1().Secrets(clusterNamespace).Get(ctx, "rook-ceph-mon", v1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get mon configmap %s %v", MonConfigMap, err)
	}

	cephFsid := string(fsidSecret.Data["fsid"])
	if cephFsid == "" {
		return fmt.Errorf("ceph cluster fsid not found")
	}

	logging.Info("printing fsid secret %s\n", cephFsid)
	logging.Info("Check for the running toolbox")

	_, err = k8sutil.GetDeployment(ctx, clientsets.Kube, clusterNamespace, "rook-ceph-tools")
	if err != nil {
		return fmt.Errorf("failed to deployment rook-ceph-tools. %v", err)
	}

	toolBox, err := k8sutil.WaitForPodToRun(ctx, clientsets.Kube, clusterNamespace, "app=rook-ceph-tools")
	if err != nil || toolBox.Name == "" {
		return fmt.Errorf("failed to get the running toolbox")
	}

	logging.Info("Restoring mon quorum to mon %s %s\n", goodMon, goodMonPublicIp)
	logging.Info("The mons to discard are: %s\n", badMons)
	logging.Info("The cluster fsid is %s\n", cephFsid)

	var answer string
	logging.Warning("Are you sure you want to restore the quorum to mon %s? If so, enter 'yes-really-restore'\n", goodMon)
	fmt.Scanf("%s", &answer)
	err = PromptToContinueOrCancel("yes-really-restore", answer)
	if err != nil {
		return fmt.Errorf("restoring the mon quorum for mon %s is cancelled. Got %s want 'yes-really-restore'", goodMon, answer)
	}
	logging.Info("proceeding with resorting quorum")

	logging.Info("Waiting for operator pod to stop")
	err = k8sutil.SetDeploymentScale(ctx, clientsets.Kube, operatorNamespace, "rook-ceph-operator", 0)
	if err != nil {
		return fmt.Errorf("failed to stop deployment rook-ceph-operator. %v", err)
	}
	logging.Info("rook-ceph-operator deployment scaled down")

	logging.Info("Waiting for bad mon pod to stop")
	for _, badMon := range badMons {
		err = k8sutil.SetDeploymentScale(ctx, clientsets.Kube, clusterNamespace, fmt.Sprintf("rook-ceph-mon-%s", badMon), 0)
		if err != nil {
			return fmt.Errorf("deployment %s still exist. %v", fmt.Sprintf("rook-ceph-mon-%s", badMon), err)
		}
		logging.Info("deployment.apps/%s scaled\n", fmt.Sprintf("rook-ceph-mon-%s", badMon))
	}

	debug.StartDebug(ctx, clientsets.Kube, clusterNamespace, fmt.Sprintf("rook-ceph-mon-%s", goodMon), "")

	debugDeploymentSpec, err := k8sutil.GetDeployment(ctx, clientsets.Kube, clusterNamespace, fmt.Sprintf("rook-ceph-mon-%s-debug", goodMon))
	if err != nil {
		return fmt.Errorf("failed to deployment rook-ceph-mon-%s-debug", goodMon)
	}

	labelSelector := fmt.Sprintf("ceph_daemon_type=%s,ceph_daemon_id=%s", debugDeploymentSpec.Spec.Template.Labels["ceph_daemon_type"], debugDeploymentSpec.Spec.Template.Labels["ceph_daemon_id"])
	_, err = k8sutil.WaitForPodToRun(ctx, clientsets.Kube, clusterNamespace, labelSelector)
	if err != nil {
		return fmt.Errorf("failed to start deployment %s", fmt.Sprintf("rook-ceph-mon-%s-debug", goodMon))
	}

	updateMonMap(ctx, clientsets, clusterNamespace, labelSelector, cephFsid, goodMon, goodMonPublicIp, badMons)

	logging.Info("Restoring the mons in the rook-ceph-mon-endpoints configmap to the good mon")
	monCm.Data["data"] = fmt.Sprintf("%s=%s:%s", goodMon, goodMonPublicIp, goodMonPort)

	_, err = clientsets.Kube.CoreV1().ConfigMaps(clusterNamespace).Update(ctx, monCm, v1.UpdateOptions{})
	if err != nil {
		logging.Error(fmt.Errorf("failed to update mon configmap %s %v", MonConfigMap, err))
	}

	logging.Info("Stopping the debug pod for mon %s.\n", goodMon)
	debug.StopDebug(ctx, clientsets.Kube, clusterNamespace, fmt.Sprintf("rook-ceph-mon-%s", goodMon))

	logging.Info("Check that the restored mon is responding")
	err = waitForMonStatusResponse(ctx, clientsets, clusterNamespace)
	if err != nil {
		return err
	}

	err = removeBadMonsResources(ctx, clientsets.Kube, clusterNamespace, badMons)
	if err != nil {
		return err
	}

	logging.Info("Mon quorum was successfully restored to mon %s\n", goodMon)
	logging.Info("Only a single mon is currently running")
	logging.Info("Enter 'continue' to start the operator and expand to full mon quorum again")

	fmt.Scanln(&answer)
	err = PromptToContinueOrCancel("continue", answer)
	if err != nil {
		return fmt.Errorf("skipping operator start to expand full mon quorum. %s", answer)
	}
	logging.Info("proceeding with resorting quorum")

	err = k8sutil.SetDeploymentScale(ctx, clientsets.Kube, operatorNamespace, "rook-ceph-operator", 1)
	if err != nil {
		return fmt.Errorf("failed to start deployment rook-ceph-operator. %v", err)
	}

	return nil
}

func updateMonMap(ctx context.Context, clientsets *k8sutil.Clientsets, clusterNamespace, labelSelector, cephFsid, goodMon, goodMonPublicIp string, badMons []string) {
	logging.Info("Started debug pod, restoring the mon quorum in the debug pod")

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

	logging.Info("Extracting the monmap")
	_, err := exec.RunCommandInLabeledPod(ctx, clientsets, labelSelector, "mon", "ceph-mon", extractMonMapArgs, clusterNamespace, false)
	if err != nil {
		logging.Fatal(err, "failed to extract monmap")
	}

	logging.Info("Printing monmap")
	_, err = exec.RunCommandInLabeledPod(ctx, clientsets, labelSelector, "mon", "monmaptool", []string{"--print", monmapPath}, clusterNamespace, false)
	if err != nil {
		logging.Fatal(err, "failed to print monmap")
	}

	// remove all the mons except the good one
	for _, badMonId := range badMons {
		logging.Info("Removing mon %s.\n", badMonId)
		_, err = exec.RunCommandInLabeledPod(ctx, clientsets, labelSelector, "mon", "monmaptool", []string{monmapPath, "--rm", badMonId}, clusterNamespace, false)
		if err != nil {
			logging.Fatal(err, "failed to remove mon %s", badMonId)
		}
	}

	injectMonMap := []string{fmt.Sprintf("--inject-monmap=%s", monmapPath)}
	injectMonMapArgs := append(monMapArgs, injectMonMap...)

	logging.Info("Injecting the monmap")
	_, err = exec.RunCommandInLabeledPod(ctx, clientsets, labelSelector, "mon", "ceph-mon", injectMonMapArgs, clusterNamespace, false)
	if err != nil {
		logging.Fatal(err, "failed to inject the monmap")
	}

	logging.Info("Finished updating the monmap!")

	logging.Info("Printing final monmap")
	_, err = exec.RunCommandInLabeledPod(ctx, clientsets, labelSelector, "mon", "monmaptool", []string{"--print", monmapPath}, clusterNamespace, false)
	if err != nil {
		logging.Fatal(err, "failed to print final monmap")
	}
}

func removeBadMonsResources(ctx context.Context, k8sclientset kubernetes.Interface, clusterNamespace string, badMons []string) error {
	logging.Info("Purging the bad mons %v\n", badMons)

	for _, badMon := range badMons {
		logging.Info("purging bad mon: %s\n", badMon)
		err := k8sclientset.AppsV1().Deployments(clusterNamespace).Delete(ctx, fmt.Sprintf("rook-ceph-mon-%s", badMon), v1.DeleteOptions{})
		if err != nil {
			return fmt.Errorf("failed to delete deployment %s", fmt.Sprintf("rook-ceph-mon-%s", badMon))
		}
		err = k8sclientset.CoreV1().Services(clusterNamespace).Delete(ctx, fmt.Sprintf("rook-ceph-mon-%s", badMon), v1.DeleteOptions{})
		if err != nil && !kerrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete service %s", fmt.Sprintf("rook-ceph-mon-%s", badMon))
		}

		err = k8sclientset.CoreV1().PersistentVolumeClaims(clusterNamespace).Delete(ctx, fmt.Sprintf("rook-ceph-mon-%s", badMon), v1.DeleteOptions{})
		if err != nil && !kerrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete pvc %s", fmt.Sprintf("rook-ceph-mon-%s", badMon))
		}
	}
	return nil
}

func wait(i int, output string) {
	logging.Info("%d: waiting for ceph status to confirm single mon quorum. \n", i+1)
	logging.Info("current ceph status output %s\n", output)
	logging.Info("sleeping for 5 seconds")
	time.Sleep(5 * time.Second)
}

func waitForMonStatusResponse(ctx context.Context, clientsets *k8sutil.Clientsets, clusterNamespace string) error {
	maxRetries := 20

	for i := 0; i < maxRetries; i++ {
		output, err := exec.RunCommandInToolboxPod(ctx, clientsets, "ceph", []string{"status"}, clusterNamespace, true)
		if err != nil {
			logging.Error(err, "failed to get the status of ceph cluster")
			wait(i, "")
			continue
		}
		if strings.Contains(output, "HEALTH_WARN") || strings.Contains(output, "HEALTH_OK") || strings.Contains(output, "HEALTH_ERROR") {
			logging.Info("finished waiting for ceph status %s\n", output)
			break
		}
		if i == maxRetries-1 {
			return fmt.Errorf("timed out waiting for mon quorum to respond")
		}
		wait(i, output)
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
		logging.Info("mon=%s, endpoints=%s\n", monName, monEndpoint)
	}
	return badMons, goodMonPublicIp, goodMonPort, nil
}

func validateMonIsUp(ctx context.Context, clientsets *k8sutil.Clientsets, clusterNamespace, monID string) error {
	args := []string{"daemon", fmt.Sprintf("mon.%s", monID), "mon_status"}
	out, err := exec.RunCommandInLabeledPod(ctx, clientsets, fmt.Sprintf("mon=%s", monID), "mon", "ceph", args, clusterNamespace, true)
	if err != nil {
		return fmt.Errorf("failed to run command ceph daemon mon.%s mon_status %v", monID, err)
	}

	type monStatus struct {
		State string `json:"state"`
	}
	var monStatusOut monStatus
	err = json.Unmarshal([]byte(out), &monStatusOut)
	if err != nil {
		return fmt.Errorf("failed to unmarshal mon stat output %v", err)
	}

	logging.Info("mon %q state is %q", monID, monStatusOut.State)

	if monStatusOut.State == "leader" || monStatusOut.State == "peon" {
		return nil
	}

	return fmt.Errorf("mon %q in %q state but must be in leader/peon state", monID, monStatusOut.State)
}

func PromptToContinueOrCancel(expectedAnswer, answer string) error {
	if skip, ok := os.LookupEnv("ROOK_PLUGIN_SKIP_PROMPTS"); ok && skip == "true" {
		logging.Info("skipped prompt since ROOK_PLUGIN_SKIP_PROMPTS=true")
		return nil
	}

	if answer == expectedAnswer {
		return nil
	}

	return fmt.Errorf("cancelled")
}
