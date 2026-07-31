/*
Copyright 2026 The Rook Authors. All rights reserved.

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
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

const mtuWarningThreshold = 8900

func isOpenShiftCluster(ctx context.Context, dynamicClient dynamic.Interface) bool {
	gvr := schema.GroupVersionResource{
		Group:    "config.openshift.io",
		Version:  "v1",
		Resource: "networks",
	}
	_, err := dynamicClient.Resource(gvr).Get(ctx, "cluster", metav1.GetOptions{})
	return err == nil
}

func checkNetworkMTUConfig(ctx context.Context, dynamicClient dynamic.Interface) CheckResult {
	result := CheckResult{
		Name:     CheckNetworkMTUConfig,
		Category: CategoryNetwork,
	}

	osMTU, osNetType, osErr := getOpenShiftClusterMTU(ctx, dynamicClient)

	if osErr == nil {
		tag := "[INFO]"
		if osMTU < mtuWarningThreshold {
			tag = "[WARN]"
		}
		result.Details = append(result.Details, fmt.Sprintf("%s OpenShift cluster network MTU: %d (network type: %s)", tag, osMTU, osNetType))
		result.Status, result.Message = evaluateClusterMTU(osMTU, mtuWarningThreshold)
	} else {
		result.Status = StatusOK
		result.Message = fmt.Sprintf("MTU information not available via cluster APIs: %v", osErr)
	}

	return result
}

func getOpenShiftClusterMTU(ctx context.Context, dynamicClient dynamic.Interface) (int, string, error) {
	gvr := schema.GroupVersionResource{
		Group:    "config.openshift.io",
		Version:  "v1",
		Resource: "networks",
	}

	obj, err := dynamicClient.Resource(gvr).Get(ctx, "cluster", metav1.GetOptions{})
	if err != nil {
		return 0, "", err
	}

	mtu, _, err := unstructured.NestedInt64(obj.Object, "status", "clusterNetworkMTU")
	if err != nil {
		return 0, "", fmt.Errorf("failed to read clusterNetworkMTU: %v", err)
	}
	netType, _, err := unstructured.NestedString(obj.Object, "status", "networkType")
	if err != nil {
		return 0, "", fmt.Errorf("failed to read networkType: %v", err)
	}

	return int(mtu), netType, nil
}

func evaluateClusterMTU(mtu, threshold int) (CheckStatus, string) {
	if mtu >= threshold {
		return StatusOK, fmt.Sprintf("Cluster network MTU %d meets recommended threshold %d", mtu, threshold)
	}
	return StatusWarning, fmt.Sprintf("Cluster network MTU %d is below recommended threshold %d", mtu, threshold)
}
