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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

var osGVR = schema.GroupVersionResource{
	Group: "config.openshift.io", Version: "v1", Resource: "networks",
}

func newDynamicClient(gvrs map[schema.GroupVersionResource]string) *dynamicfake.FakeDynamicClient {
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(), gvrs,
	)
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

func TestEvaluateClusterMTU(t *testing.T) {
	tests := []struct {
		name       string
		mtu        int
		threshold  int
		wantStatus CheckStatus
		wantMsg    string
	}{
		{"above threshold", 9000, 8900, StatusOK, "meets recommended"},
		{"at threshold", 8900, 8900, StatusOK, "meets recommended"},
		{"below threshold", 1500, 8900, StatusWarning, "below recommended"},
		{"just below", 8899, 8900, StatusWarning, "below recommended"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, msg := evaluateClusterMTU(tt.mtu, tt.threshold)
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

func TestCheckNetworkMTUConfigGoodMTU(t *testing.T) {
	client := newDynamicClient(map[schema.GroupVersionResource]string{osGVR: "NetworkList"})
	createOpenShiftNetwork(t, client, 8901, "OVNKubernetes")

	result := checkNetworkMTUConfig(context.Background(), client)
	assert.Equal(t, CheckNetworkMTUConfig, result.Name)
	assert.Equal(t, CategoryNetwork, result.Category)
	assert.Equal(t, StatusOK, result.Status)
	assert.Contains(t, result.Message, "8901")

	hasInfoMTU := false
	for _, d := range result.Details {
		if strings.Contains(d, "OpenShift cluster network MTU: 8901") {
			assert.Contains(t, d, "[INFO]")
			hasInfoMTU = true
		}
	}
	assert.True(t, hasInfoMTU, "good MTU should be tagged [INFO]")
}

func TestCheckNetworkMTUConfigLowMTU(t *testing.T) {
	client := newDynamicClient(map[schema.GroupVersionResource]string{osGVR: "NetworkList"})
	createOpenShiftNetwork(t, client, 1500, "OVNKubernetes")

	result := checkNetworkMTUConfig(context.Background(), client)
	assert.Equal(t, StatusWarning, result.Status)
	assert.Contains(t, result.Message, "below")

	hasWarnMTU := false
	for _, d := range result.Details {
		if strings.Contains(d, "OpenShift cluster network MTU: 1500") {
			assert.Contains(t, d, "[WARN]")
			hasWarnMTU = true
		}
	}
	assert.True(t, hasWarnMTU, "low MTU should be tagged [WARN]")
}

func TestCheckNetworkMTUConfigNonOpenShift(t *testing.T) {
	client := newDynamicClient(map[schema.GroupVersionResource]string{})

	result := checkNetworkMTUConfig(context.Background(), client)
	assert.Equal(t, StatusOK, result.Status)
	assert.Contains(t, result.Message, "not available")
}
