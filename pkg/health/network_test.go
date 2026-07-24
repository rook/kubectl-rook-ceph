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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rook/kubectl-rook-ceph/pkg/k8sutil"
	rookv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookfake "github.com/rook/rook/pkg/client/clientset/versioned/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

var (
	osGVR = schema.GroupVersionResource{
		Group: "config.openshift.io", Version: "v1", Resource: "networks",
	}
	nadGVR = schema.GroupVersionResource{
		Group: "k8s.cni.cncf.io", Version: "v1", Resource: "network-attachment-definitions",
	}
)

func newDynamicClient(gvrs map[schema.GroupVersionResource]string) *dynamicfake.FakeDynamicClient {
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(), gvrs,
	)
}

func createNAD(t *testing.T, client *dynamicfake.FakeDynamicClient, name, namespace, config string) {
	t.Helper()
	nad := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "k8s.cni.cncf.io/v1",
			"kind":       "NetworkAttachmentDefinition",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"config": config,
			},
		},
	}
	_, err := client.Resource(nadGVR).Namespace(namespace).Create(context.Background(), nad, metav1.CreateOptions{})
	require.NoError(t, err)
}

func createOpenShiftNetwork(t *testing.T, client *dynamicfake.FakeDynamicClient, mtu int64, networkType string) {
	t.Helper()
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "config.openshift.io/v1",
			"kind":       "Network",
			"metadata":   map[string]interface{}{"name": "cluster"},
			"status": map[string]interface{}{
				"clusterNetworkMTU": mtu,
				"networkType":       networkType,
			},
		},
	}
	_, err := client.Resource(osGVR).Create(context.Background(), obj, metav1.CreateOptions{})
	require.NoError(t, err)
}

func TestExtractMTU(t *testing.T) {
	tests := []struct {
		name     string
		config   string
		expected int
	}{
		{"empty string", "", 0},
		{"invalid JSON", "{bad", 0},
		{"top-level MTU", `{"type":"macvlan","mtu":9000}`, 9000},
		{"no MTU field", `{"type":"macvlan"}`, 0},
		{"MTU zero", `{"type":"macvlan","mtu":0}`, 0},
		{"plugin chain with MTU", `{"plugins":[{"type":"macvlan"},{"type":"tuning","mtu":9000}]}`, 9000},
		{"plugin chain no MTU", `{"plugins":[{"type":"macvlan"},{"type":"tuning"}]}`, 0},
		{"both top-level and plugin, top-level wins", `{"mtu":8000,"plugins":[{"mtu":9000}]}`, 8000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, extractMTU(tt.config))
		})
	}
}

func TestExtractMaster(t *testing.T) {
	tests := []struct {
		name     string
		config   string
		expected string
	}{
		{"empty string", "", ""},
		{"invalid JSON", "{bad", ""},
		{"top-level master", `{"type":"macvlan","master":"ens224"}`, "ens224"},
		{"no master field", `{"type":"macvlan"}`, ""},
		{"plugin chain with master", `{"plugins":[{"type":"macvlan","master":"eth0"}]}`, "eth0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, extractMaster(tt.config))
		})
	}
}

func TestIsOverlayNetwork(t *testing.T) {
	tests := []struct {
		networkType string
		expected    bool
	}{
		{"OVNKubernetes", true},
		{"OpenShiftSDN", true},
		{"Calico", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.networkType, func(t *testing.T) {
			assert.Equal(t, tt.expected, isOverlayNetwork(tt.networkType))
		})
	}
}

func TestEvaluateMTU(t *testing.T) {
	tests := []struct {
		name       string
		sources    []mtuSource
		netType    string
		cephNet    cephNetworkInfo
		wantStatus CheckStatus
		wantMsg    string
	}{
		{
			name:       "no sources",
			sources:    nil,
			wantStatus: StatusOK,
			wantMsg:    "MTU information not available via cluster APIs",
		},
		{
			name:       "single source above 9000",
			sources:    []mtuSource{{Label: "test", MTU: 9000}},
			wantStatus: StatusOK,
			wantMsg:    "Network MTU 9000 consistent across 1 source(s)",
		},
		{
			name:       "single source below 9000",
			sources:    []mtuSource{{Label: "test", MTU: 1500}},
			wantStatus: StatusWarning,
			wantMsg:    "1 MTU source(s) below 9000",
		},
		{
			name: "consistent MTUs above threshold",
			sources: []mtuSource{
				{Label: "a", MTU: 9000},
				{Label: "b", MTU: 9000},
			},
			wantStatus: StatusOK,
			wantMsg:    "Network MTU 9000 consistent across 2 source(s)",
		},
		{
			name: "inconsistent MTUs",
			sources: []mtuSource{
				{Label: "a", MTU: 9000},
				{Label: "b", MTU: 1500},
			},
			wantStatus: StatusWarning,
			wantMsg:    "1 MTU source(s) below 9000, inconsistent MTU values detected",
		},
		{
			name:       "overlay adjusted threshold, 8901 is OK",
			sources:    []mtuSource{{Label: "cluster", MTU: 8901}},
			netType:    "OVNKubernetes",
			cephNet:    cephNetworkInfo{IsMultus: false},
			wantStatus: StatusOK,
			wantMsg:    "Network MTU 8901 consistent across 1 source(s)",
		},
		{
			name:       "overlay adjusted threshold, 8899 is warning",
			sources:    []mtuSource{{Label: "cluster", MTU: 8899}},
			netType:    "OVNKubernetes",
			cephNet:    cephNetworkInfo{IsMultus: false},
			wantStatus: StatusWarning,
			wantMsg:    "1 MTU source(s) below 8900",
		},
		{
			name:       "multus with overlay uses 9000 threshold",
			sources:    []mtuSource{{Label: "nad", MTU: 8950}},
			netType:    "OVNKubernetes",
			cephNet:    cephNetworkInfo{IsMultus: true},
			wantStatus: StatusWarning,
			wantMsg:    "1 MTU source(s) below 9000",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, msg, _ := evaluateMTU(tt.sources, tt.netType, tt.cephNet)
			assert.Equal(t, tt.wantStatus, status)
			assert.Contains(t, msg, tt.wantMsg)
		})
	}
}

func TestGetOpenShiftClusterMTU(t *testing.T) {
	t.Run("OpenShift present", func(t *testing.T) {
		client := newDynamicClient(map[schema.GroupVersionResource]string{osGVR: "NetworkList"})
		createOpenShiftNetwork(t, client, 8901, "OVNKubernetes")

		mtu, netType, err := getOpenShiftClusterMTU(context.Background(), client)
		require.NoError(t, err)
		assert.Equal(t, 8901, mtu)
		assert.Equal(t, "OVNKubernetes", netType)
	})

	t.Run("non-OpenShift", func(t *testing.T) {
		client := newDynamicClient(map[schema.GroupVersionResource]string{})
		_, _, err := getOpenShiftClusterMTU(context.Background(), client)
		assert.Error(t, err)
	})
}

func TestGetCephClusterNetworkConfig(t *testing.T) {
	t.Run("default provider", func(t *testing.T) {
		cluster := &rookv1.CephCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "rook-ceph", Namespace: "rook-ceph"},
		}
		client := rookfake.NewSimpleClientset(cluster)
		info, err := getCephClusterNetworkConfig(context.Background(), client, "rook-ceph")
		require.NoError(t, err)
		assert.Equal(t, "default", info.Provider)
		assert.False(t, info.IsMultus)
	})

	t.Run("multus with cross-namespace selectors", func(t *testing.T) {
		cluster := &rookv1.CephCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "rook-ceph", Namespace: "openshift-storage"},
			Spec: rookv1.ClusterSpec{
				Network: rookv1.NetworkSpec{
					Provider: rookv1.NetworkProviderMultus,
					Selectors: map[rookv1.CephNetworkType]string{
						"public":  "default/public-net",
						"cluster": "private-net",
					},
				},
			},
		}
		client := rookfake.NewSimpleClientset(cluster)
		info, err := getCephClusterNetworkConfig(context.Background(), client, "openshift-storage")
		require.NoError(t, err)
		assert.Equal(t, "multus", info.Provider)
		assert.True(t, info.IsMultus)
		assert.Equal(t, nadRef{Namespace: "default", Name: "public-net"}, info.Selectors["public"])
		assert.Equal(t, nadRef{Namespace: "openshift-storage", Name: "private-net"}, info.Selectors["cluster"])
	})

	t.Run("no CephCluster", func(t *testing.T) {
		client := rookfake.NewSimpleClientset()
		_, err := getCephClusterNetworkConfig(context.Background(), client, "rook-ceph")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no CephCluster found")
	})
}

func TestGetNADMTUs(t *testing.T) {
	t.Run("NAD with MTU in cross-namespace", func(t *testing.T) {
		client := newDynamicClient(map[schema.GroupVersionResource]string{nadGVR: "NetworkAttachmentDefinitionList"})
		createNAD(t, client, "public-net", "default", `{"type":"macvlan","master":"ens224","mtu":9000}`)

		selectors := map[string]nadRef{
			"public": {Namespace: "default", Name: "public-net"},
		}
		sources, _, items := getNADMTUs(context.Background(), client, selectors)
		require.Len(t, sources, 1)
		assert.Equal(t, 9000, sources[0].MTU)
		assert.Contains(t, sources[0].Label, "default/public-net")
		assert.Contains(t, sources[0].Label, "role=public")
		require.Len(t, items, 1)
		assert.Equal(t, "default/public-net", items[0].Name)
	})

	t.Run("NAD without MTU reports host NIC in item", func(t *testing.T) {
		client := newDynamicClient(map[schema.GroupVersionResource]string{nadGVR: "NetworkAttachmentDefinitionList"})
		createNAD(t, client, "private-net", "default", `{"type":"macvlan","master":"ens224"}`)

		selectors := map[string]nadRef{
			"cluster": {Namespace: "default", Name: "private-net"},
		}
		sources, details, items := getNADMTUs(context.Background(), client, selectors)
		assert.Empty(t, sources)
		assert.Empty(t, details)
		require.Len(t, items, 1)
		assert.Equal(t, "default/private-net", items[0].Name)
		assert.Contains(t, items[0].Details, "MTU not specified")
		assert.Contains(t, items[0].Details, "ens224")
		assert.Contains(t, items[0].Details, "role=cluster")
	})

	t.Run("NAD without MTU or master", func(t *testing.T) {
		client := newDynamicClient(map[schema.GroupVersionResource]string{nadGVR: "NetworkAttachmentDefinitionList"})
		createNAD(t, client, "simple-net", "default", `{"type":"bridge"}`)

		selectors := map[string]nadRef{
			"public": {Namespace: "default", Name: "simple-net"},
		}
		_, details, items := getNADMTUs(context.Background(), client, selectors)
		assert.Empty(t, details)
		require.Len(t, items, 1)
		assert.Contains(t, items[0].Details, "inherits from host NIC")
		assert.NotContains(t, items[0].Details, "NIC ens")
	})

	t.Run("missing referenced NAD", func(t *testing.T) {
		client := newDynamicClient(map[schema.GroupVersionResource]string{nadGVR: "NetworkAttachmentDefinitionList"})

		selectors := map[string]nadRef{
			"public": {Namespace: "default", Name: "missing-nad"},
		}
		_, details, _ := getNADMTUs(context.Background(), client, selectors)
		require.Len(t, details, 1)
		assert.Contains(t, details[0], "default/missing-nad")
		assert.Contains(t, details[0], "not found")
	})
}

func TestCheckNetworkMTUConfigOpenShiftNoMultus(t *testing.T) {
	dynClient := newDynamicClient(map[schema.GroupVersionResource]string{
		osGVR:  "NetworkList",
		nadGVR: "NetworkAttachmentDefinitionList",
	})
	createOpenShiftNetwork(t, dynClient, 8901, "OVNKubernetes")

	cluster := &rookv1.CephCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "rook-ceph", Namespace: "rook-ceph"},
	}
	rookClient := rookfake.NewSimpleClientset(cluster)

	clientsets := &k8sutil.Clientsets{
		Dynamic: dynClient,
		Rook:    rookClient,
	}

	result := checkNetworkMTUConfig(context.Background(), clientsets, "rook-ceph")
	assert.Equal(t, CheckNetworkMTUConfig, result.Name)
	assert.Equal(t, CategoryNetwork, result.Category)
	assert.Equal(t, StatusOK, result.Status)
	assert.Contains(t, result.Message, "8901")
}

func TestCheckNetworkMTUConfigLowMTU(t *testing.T) {
	dynClient := newDynamicClient(map[schema.GroupVersionResource]string{
		osGVR:  "NetworkList",
		nadGVR: "NetworkAttachmentDefinitionList",
	})
	createOpenShiftNetwork(t, dynClient, 1500, "OVNKubernetes")

	cluster := &rookv1.CephCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "rook-ceph", Namespace: "rook-ceph"},
	}
	rookClient := rookfake.NewSimpleClientset(cluster)

	clientsets := &k8sutil.Clientsets{
		Dynamic: dynClient,
		Rook:    rookClient,
	}

	result := checkNetworkMTUConfig(context.Background(), clientsets, "rook-ceph")
	assert.Equal(t, StatusWarning, result.Status)
	assert.Contains(t, result.Message, "below")
}

func TestCheckNetworkMTUConfigNonOpenShift(t *testing.T) {
	dynClient := newDynamicClient(map[schema.GroupVersionResource]string{
		nadGVR: "NetworkAttachmentDefinitionList",
	})

	cluster := &rookv1.CephCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "rook-ceph", Namespace: "rook-ceph"},
	}
	rookClient := rookfake.NewSimpleClientset(cluster)

	clientsets := &k8sutil.Clientsets{
		Dynamic: dynClient,
		Rook:    rookClient,
	}

	result := checkNetworkMTUConfig(context.Background(), clientsets, "rook-ceph")
	assert.Equal(t, StatusOK, result.Status)
	assert.Contains(t, result.Message, "not available")
}

func TestCheckNetworkMTUConfigMultusCrossNamespace(t *testing.T) {
	dynClient := newDynamicClient(map[schema.GroupVersionResource]string{
		osGVR:  "NetworkList",
		nadGVR: "NetworkAttachmentDefinitionList",
	})
	createOpenShiftNetwork(t, dynClient, 9000, "OVNKubernetes")
	createNAD(t, dynClient, "public-net", "default", `{"type":"macvlan","master":"ens224"}`)
	createNAD(t, dynClient, "private-net", "default", `{"type":"macvlan","master":"ens224"}`)

	cluster := &rookv1.CephCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "ocs-storagecluster-cephcluster", Namespace: "openshift-storage"},
		Spec: rookv1.ClusterSpec{
			Network: rookv1.NetworkSpec{
				Provider: rookv1.NetworkProviderMultus,
				Selectors: map[rookv1.CephNetworkType]string{
					"public":  "default/public-net",
					"cluster": "default/private-net",
				},
			},
		},
	}
	rookClient := rookfake.NewSimpleClientset(cluster)

	clientsets := &k8sutil.Clientsets{
		Dynamic: dynClient,
		Rook:    rookClient,
	}

	result := checkNetworkMTUConfig(context.Background(), clientsets, "openshift-storage")
	assert.Equal(t, StatusOK, result.Status)
	assert.Contains(t, result.Message, "9000")

	hasPublicItem := false
	hasPrivateItem := false
	for _, item := range result.Items {
		if item.Name == "default/public-net" && strings.Contains(item.Details, "MTU not specified") && strings.Contains(item.Details, "ens224") {
			hasPublicItem = true
		}
		if item.Name == "default/private-net" && strings.Contains(item.Details, "MTU not specified") && strings.Contains(item.Details, "ens224") {
			hasPrivateItem = true
		}
	}
	assert.True(t, hasPublicItem, "should report public NAD MTU not specified with host NIC in items")
	assert.True(t, hasPrivateItem, "should report private NAD MTU not specified with host NIC in items")
}
