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
	"regexp"

	"github.com/rook/kubectl-rook-ceph/pkg/k8sutil"
	"github.com/rook/kubectl-rook-ceph/pkg/logging"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const MonConfigMap = "rook-ceph-mon-endpoints"

func GetMonEndpoint(context *k8sutil.Context, clusterNamespace string) string {
	monCm, err := context.Clientset.CoreV1().ConfigMaps(clusterNamespace).Get(ctx.TODO(), MonConfigMap, v1.GetOptions{})
	if err != nil {
		logging.Error(fmt.Errorf("failed to get mon configmap %s %v", MonConfigMap, err))
	}

	monData := monCm.Data["data"]
	reg, err := regexp.Compile("[^0-9,.:]+")
	if err != nil {
		logging.Fatal(err)
	}
	return reg.ReplaceAllLiteralString(monData, "")
}
