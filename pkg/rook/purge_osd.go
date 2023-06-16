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

package rook

import (
	"context"
	"fmt"

	exec "github.com/rook/kubectl-rook-ceph/pkg/exec"
	"github.com/rook/kubectl-rook-ceph/pkg/k8sutil"
	"github.com/rook/kubectl-rook-ceph/pkg/logging"
	"github.com/rook/kubectl-rook-ceph/pkg/mons"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func PurgeOsd(ctx context.Context, clientsets *k8sutil.Clientsets, operatorNamespace, clusterNamespace, osdId, flag string) {
	monCm, err := clientsets.Kube.CoreV1().ConfigMaps(clusterNamespace).Get(ctx, mons.MonConfigMap, v1.GetOptions{})
	if err != nil {
		logging.Fatal(fmt.Errorf("failed to get mon configmap %s %v", mons.MonConfigMap, err))
	}

	monEndPoint := monCm.Data["data"]
	cephArgs := []string{
		"auth", "print-key", "client.admin",
	}

	adminKey := exec.RunCommandInOperatorPod(ctx, clientsets, "ceph", cephArgs, operatorNamespace, clusterNamespace, true, false)
	if adminKey == "" {
		logging.Fatal(fmt.Errorf("failed to get ceph key"))
	}

	cmd := "/bin/sh"
	args := []string{
		"-c",
		fmt.Sprintf("export ROOK_MON_ENDPOINTS=%s ROOK_CEPH_USERNAME=client.admin ROOK_CEPH_SECRET=%s ROOK_CONFIG_DIR=/var/lib/rook && rook ceph osd remove --osd-ids=%s --force-osd-removal=%s", monEndPoint, adminKey, osdId, flag),
	}
	logging.Info("Running purge osd command")

	exec.RunCommandInOperatorPod(ctx, clientsets, cmd, args, operatorNamespace, clusterNamespace, false, true)
}
