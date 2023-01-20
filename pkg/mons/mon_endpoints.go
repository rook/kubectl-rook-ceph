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
	"fmt"
	"regexp"

	"github.com/rook/kubectl-rook-ceph/pkg/k8sutil"
	log "github.com/sirupsen/logrus"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const monConfigMap = "rook-ceph-mon-endpoints"

func GetMonEndpoint(clusterNamespace string) {
	// Get Kubernetes Client
	_, _, client := k8sutil.GetKubeClient()

	monCm, err := client.ConfigMaps(clusterNamespace).Get(context.TODO(), monConfigMap, v1.GetOptions{})
	if err != nil {
		log.Fatalf("failed to get mon configmap %s %v", monConfigMap, err)
	}

	monData := monCm.Data["data"]
	fmt.Println(parseMonEndpoint(monData))
}

func parseMonEndpoint(monData string) string {
	reg, err := regexp.Compile("[^0-9,.:]+")
	if err != nil {
		log.Fatal(err)
	}
	return reg.ReplaceAllString(monData, "")
}
