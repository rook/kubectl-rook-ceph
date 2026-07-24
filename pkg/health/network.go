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
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rook/kubectl-rook-ceph/pkg/k8sutil"

	rookclient "github.com/rook/rook/pkg/client/clientset/versioned"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

type mtuSource struct {
	Label string
	MTU   int
}

type nadRef struct {
	Namespace string
	Name      string
}

type cephNetworkInfo struct {
	Provider  string
	Selectors map[string]nadRef
	IsMultus  bool
}

func checkNetworkMTUConfig(ctx context.Context, clientsets *k8sutil.Clientsets, clusterNamespace string) CheckResult {
	result := CheckResult{
		Name:     CheckNetworkMTUConfig,
		Category: CategoryNetwork,
	}

	var sources []mtuSource
	var networkType string

	osMTU, osNetType, err := getOpenShiftClusterMTU(ctx, clientsets.Dynamic)
	if err != nil {
		result.Details = append(result.Details, "OpenShift network config not available (non-OpenShift cluster)")
	} else {
		networkType = osNetType
		result.Details = append(result.Details, fmt.Sprintf("OpenShift cluster network MTU: %d (network type: %s)", osMTU, osNetType))
		sources = append(sources, mtuSource{Label: "OpenShift cluster network", MTU: osMTU})
	}

	cephNet, err := getCephClusterNetworkConfig(ctx, clientsets.Rook, clusterNamespace)
	if err != nil {
		result.Details = append(result.Details, fmt.Sprintf("Could not read CephCluster: %v", err))
	} else {
		result.Details = append(result.Details, fmt.Sprintf("CephCluster network provider: %s", cephNet.Provider))
	}

	if cephNet.IsMultus {
		nadSources, nadDetails, nadItems := getNADMTUs(ctx, clientsets.Dynamic, cephNet.Selectors)
		sources = append(sources, nadSources...)
		result.Details = append(result.Details, nadDetails...)
		result.Items = append(result.Items, nadItems...)
	}

	status, message, extraDetails := evaluateMTU(sources, networkType, cephNet)
	result.Status = status
	result.Message = message
	result.Details = append(result.Details, extraDetails...)

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

	mtu, _, _ := unstructured.NestedInt64(obj.Object, "status", "clusterNetworkMTU")
	netType, _, _ := unstructured.NestedString(obj.Object, "status", "networkType")

	return int(mtu), netType, nil
}

func getCephClusterNetworkConfig(ctx context.Context, rookClient rookclient.Interface, namespace string) (cephNetworkInfo, error) {
	clusters, err := rookClient.CephV1().CephClusters(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return cephNetworkInfo{}, fmt.Errorf("failed to list CephClusters: %v", err)
	}

	if len(clusters.Items) == 0 {
		return cephNetworkInfo{}, fmt.Errorf("no CephCluster found in namespace %s", namespace)
	}

	cluster := clusters.Items[0]
	provider := string(cluster.Spec.Network.Provider)
	if provider == "" {
		provider = "default"
	}

	selectors := make(map[string]nadRef)
	for role, ref := range cluster.Spec.Network.Selectors {
		val := string(ref)
		r := nadRef{Namespace: namespace, Name: val}
		if parts := strings.SplitN(val, "/", 2); len(parts) == 2 {
			r.Namespace = parts[0]
			r.Name = parts[1]
		}
		selectors[string(role)] = r
	}

	return cephNetworkInfo{
		Provider:  provider,
		Selectors: selectors,
		IsMultus:  cluster.Spec.Network.IsMultus(),
	}, nil
}

func getNADMTUs(ctx context.Context, dynamicClient dynamic.Interface, selectors map[string]nadRef) ([]mtuSource, []string, []CheckItem) {
	gvr := schema.GroupVersionResource{
		Group:    "k8s.cni.cncf.io",
		Version:  "v1",
		Resource: "network-attachment-definitions",
	}

	var sources []mtuSource
	var details []string
	var items []CheckItem

	for role, ref := range selectors {
		qualifiedName := fmt.Sprintf("%s/%s", ref.Namespace, ref.Name)
		obj, err := dynamicClient.Resource(gvr).Namespace(ref.Namespace).Get(ctx, ref.Name, metav1.GetOptions{})
		if err != nil {
			details = append(details, fmt.Sprintf("NAD %s (role=%s): not found", qualifiedName, role))
			continue
		}

		config, _, _ := unstructured.NestedString(obj.Object, "spec", "config")
		mtu := extractMTU(config)

		if mtu <= 0 {
			master := extractMaster(config)
			detail := "MTU not specified, inherits from host NIC"
			if master != "" {
				detail = fmt.Sprintf("MTU not specified, inherits from host NIC %s", master)
			}
			items = append(items, CheckItem{
				Name:    qualifiedName,
				Details: fmt.Sprintf("role=%s, %s", role, detail),
			})
			continue
		}

		label := fmt.Sprintf("NAD %s (role=%s)", qualifiedName, role)
		sources = append(sources, mtuSource{Label: label, MTU: mtu})
		items = append(items, CheckItem{
			Name:    qualifiedName,
			Details: fmt.Sprintf("role=%s, MTU %d", role, mtu),
		})
	}

	return sources, details, items
}

func extractMTU(configJSON string) int {
	if configJSON == "" {
		return 0
	}

	var config map[string]interface{}
	if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
		return 0
	}

	if mtu, ok := config["mtu"].(float64); ok && mtu > 0 {
		return int(mtu)
	}

	plugins, ok := config["plugins"].([]interface{})
	if !ok {
		return 0
	}
	for _, p := range plugins {
		plugin, ok := p.(map[string]interface{})
		if !ok {
			continue
		}
		if mtu, ok := plugin["mtu"].(float64); ok && mtu > 0 {
			return int(mtu)
		}
	}

	return 0
}

func extractMaster(configJSON string) string {
	if configJSON == "" {
		return ""
	}

	var config map[string]interface{}
	if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
		return ""
	}

	if master, ok := config["master"].(string); ok {
		return master
	}

	plugins, ok := config["plugins"].([]interface{})
	if !ok {
		return ""
	}
	for _, p := range plugins {
		plugin, ok := p.(map[string]interface{})
		if !ok {
			continue
		}
		if master, ok := plugin["master"].(string); ok {
			return master
		}
	}

	return ""
}

func evaluateMTU(sources []mtuSource, networkType string, cephNet cephNetworkInfo) (CheckStatus, string, []string) {
	if len(sources) == 0 {
		return StatusOK, "MTU information not available via cluster APIs", nil
	}

	threshold := 9000
	if isOverlayNetwork(networkType) && !cephNet.IsMultus {
		threshold = 8900
	}

	belowCount := 0
	mtuValues := make(map[int]bool)
	for _, s := range sources {
		mtuValues[s.MTU] = true
		if s.MTU < threshold {
			belowCount++
		}
	}

	consistent := len(mtuValues) == 1
	var details []string

	if belowCount == 0 && consistent {
		var mtu int
		for m := range mtuValues {
			mtu = m
		}
		return StatusOK, fmt.Sprintf("Network MTU %d consistent across %d source(s)", mtu, len(sources)), details
	}

	status := StatusWarning
	var parts []string
	if belowCount > 0 {
		parts = append(parts, fmt.Sprintf("%d MTU source(s) below %d", belowCount, threshold))
	}
	if !consistent {
		parts = append(parts, "inconsistent MTU values detected")
		for _, s := range sources {
			details = append(details, fmt.Sprintf("%s MTU: %d", s.Label, s.MTU))
		}
	}

	return status, strings.Join(parts, ", "), details
}

func isOverlayNetwork(networkType string) bool {
	return networkType == "OVNKubernetes" || networkType == "OpenShiftSDN"
}
