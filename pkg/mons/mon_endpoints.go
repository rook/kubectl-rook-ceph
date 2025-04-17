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
	"net"
	"regexp"

	"github.com/rook/kubectl-rook-ceph/pkg/logging"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const MonConfigMap = "rook-ceph-mon-endpoints"

func ParseMonEndpoint(monEndpoint string) (string, string, error) {
	host, port, err := net.SplitHostPort(monEndpoint)
	if err != nil {
		return "", "", fmt.Errorf("failed to split host and port from endpoint %s: %v", monEndpoint, err)
	}

	if ip := net.ParseIP(host); ip == nil {
		return "", "", fmt.Errorf("invalid IP address in endpoint: %s", host)
	}

	return host, port, nil
}

func GetMonEndpoint(ctx context.Context, k8sclientset kubernetes.Interface, clusterNamespace string) string {
	monCm, err := k8sclientset.CoreV1().ConfigMaps(clusterNamespace).Get(ctx, MonConfigMap, v1.GetOptions{})
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
