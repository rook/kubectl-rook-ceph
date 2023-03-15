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
	ctx "context"
	"fmt"
	"log"

	"github.com/rook/kubectl-rook-ceph/pkg/exec"
	"github.com/rook/kubectl-rook-ceph/pkg/k8sutil"
	"github.com/rook/kubectl-rook-ceph/pkg/mons"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func PurgeOsd(context *k8sutil.Context, operatorNamespace, clusterNamespace, osdId, flag string) string {
	monCm, err := context.Clientset.CoreV1().ConfigMaps(clusterNamespace).Get(ctx.TODO(), mons.MonConfigMap, v1.GetOptions{})
	if err != nil {
		log.Fatalf("failed to get mon configmap %s %v", mons.MonConfigMap, err)
	}

	monEndPoint := monCm.Data["data"]
	cephArgs := []string{
		"auth", "print-key", "client.admin",
	}
	adminKey := exec.RunCommandInOperatorPod(context, "ceph", cephArgs, operatorNamespace, clusterNamespace, true)

	cmd := "/bin/sh"
	args := []string{
		"-c",
		fmt.Sprintf("export ROOK_MON_ENDPOINTS=%s ROOK_CEPH_USERNAME=client.admin ROOK_CEPH_SECRET=%s ROOK_CONFIG_DIR=/var/lib/rook && rook ceph osd remove --osd-ids=%s --force-osd-removal=%s", monEndPoint, adminKey, osdId, flag),
	}

	return exec.RunCommandInOperatorPod(context, cmd, args, operatorNamespace, clusterNamespace, true)
}
