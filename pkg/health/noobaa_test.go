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
	"testing"
	"time"

	nbv1 "github.com/noobaa/noobaa-operator/v5/pkg/apis/noobaa/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
)

const testNamespace = "rook-ceph"

func createNooBaaCR(t *testing.T, client *dynamicfake.FakeDynamicClient, phase string, creationTime time.Time) {
	t.Helper()
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "noobaa.io/v1alpha1",
			"kind":       "NooBaa",
			"metadata": map[string]interface{}{
				"name":              "noobaa",
				"namespace":         testNamespace,
				"creationTimestamp": creationTime.UTC().Format(time.RFC3339),
			},
			"status": map[string]interface{}{
				"phase": phase,
			},
		},
	}
	_, err := client.Resource(noobaaGVR).Namespace(testNamespace).Create(context.Background(), obj, metav1.CreateOptions{})
	require.NoError(t, err)
}

func noobaaCorePod(name string, phase v1.PodPhase, containerStatuses []v1.ContainerStatus) *v1.Pod {
	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
			Labels:    map[string]string{"noobaa-core": "noobaa"},
		},
		Spec: v1.PodSpec{
			NodeName: "worker-1",
		},
		Status: v1.PodStatus{
			Phase:             phase,
			ContainerStatuses: containerStatuses,
		},
	}
}

func TestIsContainerCrashLooping(t *testing.T) {
	t.Run("not crashing", func(t *testing.T) {
		pod := noobaaCorePod("noobaa-core-0", v1.PodRunning, []v1.ContainerStatus{
			{Name: "core", State: v1.ContainerState{Running: &v1.ContainerStateRunning{}}},
		})
		name, crashing := isContainerCrashLooping(pod)
		assert.False(t, crashing)
		assert.Empty(t, name)
	})

	t.Run("crash loop", func(t *testing.T) {
		pod := noobaaCorePod("noobaa-core-0", v1.PodRunning, []v1.ContainerStatus{
			{Name: "core", State: v1.ContainerState{Waiting: &v1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}}},
		})
		name, crashing := isContainerCrashLooping(pod)
		assert.True(t, crashing)
		assert.Equal(t, "core", name)
	})
}

func TestFormatDuration(t *testing.T) {
	assert.Equal(t, "30s", formatDuration(30*time.Second))
	assert.Equal(t, "1m30s", formatDuration(90*time.Second))
	assert.Equal(t, "8m0s", formatDuration(8*time.Minute))
}

func TestCheckNooBaaHealth(t *testing.T) {
	noobaaGVRs := map[schema.GroupVersionResource]string{noobaaGVR: "NooBaaList"}

	t.Run("no NooBaa CR present", func(t *testing.T) {
		dynClient := newDynamicClient(noobaaGVRs)
		k8sClient := fake.NewSimpleClientset()

		result := checkNooBaaHealth(context.Background(), dynClient, k8sClient, testNamespace)
		assert.Empty(t, result.Name, "should return empty result when no NooBaa exists")
	})

	t.Run("phase Ready with running pod", func(t *testing.T) {
		dynClient := newDynamicClient(noobaaGVRs)
		createNooBaaCR(t, dynClient, string(nbv1.SystemPhaseReady), time.Now().Add(-1*time.Hour))
		k8sClient := fake.NewSimpleClientset(
			noobaaCorePod("noobaa-core-0", v1.PodRunning, []v1.ContainerStatus{
				{Name: "core", State: v1.ContainerState{Running: &v1.ContainerStateRunning{}}},
			}),
		)

		result := checkNooBaaHealth(context.Background(), dynClient, k8sClient, testNamespace)
		assert.Equal(t, CheckNooBaaHealth, result.Name)
		assert.Equal(t, CategoryObjectStorage, result.Category)
		assert.Equal(t, StatusOK, result.Status)
		assert.Contains(t, result.Message, "Ready")

		joined := fmt.Sprintf("%v", result.Details)
		assert.Contains(t, joined, "[INFO] Phase: Ready")
		assert.Contains(t, joined, "[INFO] Core pod noobaa-core-0 is Running")

		require.Len(t, result.Items, 1)
		assert.Equal(t, "noobaa-core-0", result.Items[0].Name)
		assert.Equal(t, "Running", result.Items[0].Status)
	})

	t.Run("phase Creating under 5 minutes", func(t *testing.T) {
		dynClient := newDynamicClient(noobaaGVRs)
		createNooBaaCR(t, dynClient, string(nbv1.SystemPhaseCreating), time.Now().Add(-2*time.Minute))
		k8sClient := fake.NewSimpleClientset(
			noobaaCorePod("noobaa-core-0", v1.PodPending, nil),
		)

		result := checkNooBaaHealth(context.Background(), dynClient, k8sClient, testNamespace)
		assert.Equal(t, StatusCritical, result.Status)

		joined := fmt.Sprintf("%v", result.Details)
		assert.Contains(t, joined, "[INFO] Phase: Creating")
	})

	t.Run("phase Creating over 5 minutes", func(t *testing.T) {
		dynClient := newDynamicClient(noobaaGVRs)
		createNooBaaCR(t, dynClient, string(nbv1.SystemPhaseCreating), time.Now().Add(-8*time.Minute))
		k8sClient := fake.NewSimpleClientset(
			noobaaCorePod("noobaa-core-0", v1.PodPending, nil),
		)

		result := checkNooBaaHealth(context.Background(), dynClient, k8sClient, testNamespace)
		assert.Equal(t, StatusCritical, result.Status)

		joined := fmt.Sprintf("%v", result.Details)
		assert.Contains(t, joined, "[WARN] Phase: Creating")
	})

	t.Run("phase Rejected", func(t *testing.T) {
		dynClient := newDynamicClient(noobaaGVRs)
		createNooBaaCR(t, dynClient, string(nbv1.SystemPhaseRejected), time.Now().Add(-10*time.Minute))
		k8sClient := fake.NewSimpleClientset(
			noobaaCorePod("noobaa-core-0", v1.PodRunning, []v1.ContainerStatus{
				{Name: "core", State: v1.ContainerState{Running: &v1.ContainerStateRunning{}}},
			}),
		)

		result := checkNooBaaHealth(context.Background(), dynClient, k8sClient, testNamespace)
		assert.Equal(t, StatusCritical, result.Status)

		joined := fmt.Sprintf("%v", result.Details)
		assert.Contains(t, joined, "[ERR] Phase: Rejected")
	})

	t.Run("phase Connecting over 5 minutes", func(t *testing.T) {
		dynClient := newDynamicClient(noobaaGVRs)
		createNooBaaCR(t, dynClient, string(nbv1.SystemPhaseConnecting), time.Now().Add(-6*time.Minute))
		k8sClient := fake.NewSimpleClientset(
			noobaaCorePod("noobaa-core-0", v1.PodRunning, []v1.ContainerStatus{
				{Name: "core", State: v1.ContainerState{Running: &v1.ContainerStateRunning{}}},
			}),
		)

		result := checkNooBaaHealth(context.Background(), dynClient, k8sClient, testNamespace)
		assert.Equal(t, StatusWarning, result.Status)

		joined := fmt.Sprintf("%v", result.Details)
		assert.Contains(t, joined, "[WARN] Phase: Connecting")
	})

	t.Run("phase Verifying under 5 minutes", func(t *testing.T) {
		dynClient := newDynamicClient(noobaaGVRs)
		createNooBaaCR(t, dynClient, string(nbv1.SystemPhaseVerifying), time.Now().Add(-1*time.Minute))
		k8sClient := fake.NewSimpleClientset(
			noobaaCorePod("noobaa-core-0", v1.PodRunning, []v1.ContainerStatus{
				{Name: "core", State: v1.ContainerState{Running: &v1.ContainerStateRunning{}}},
			}),
		)

		result := checkNooBaaHealth(context.Background(), dynClient, k8sClient, testNamespace)
		assert.Equal(t, StatusOK, result.Status)

		joined := fmt.Sprintf("%v", result.Details)
		assert.Contains(t, joined, "[INFO] Phase: Verifying")
	})

	t.Run("core pod CrashLoopBackOff", func(t *testing.T) {
		dynClient := newDynamicClient(noobaaGVRs)
		createNooBaaCR(t, dynClient, string(nbv1.SystemPhaseReady), time.Now().Add(-1*time.Hour))
		k8sClient := fake.NewSimpleClientset(
			noobaaCorePod("noobaa-core-0", v1.PodRunning, []v1.ContainerStatus{
				{Name: "core", State: v1.ContainerState{Waiting: &v1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}}},
			}),
		)

		result := checkNooBaaHealth(context.Background(), dynClient, k8sClient, testNamespace)
		assert.Equal(t, StatusCritical, result.Status)

		joined := fmt.Sprintf("%v", result.Details)
		assert.Contains(t, joined, "[ERR] Core pod noobaa-core-0: container core in CrashLoopBackOff")
	})

	t.Run("no core pods found", func(t *testing.T) {
		dynClient := newDynamicClient(noobaaGVRs)
		createNooBaaCR(t, dynClient, string(nbv1.SystemPhaseReady), time.Now().Add(-1*time.Hour))
		k8sClient := fake.NewSimpleClientset()

		result := checkNooBaaHealth(context.Background(), dynClient, k8sClient, testNamespace)
		assert.Equal(t, StatusWarning, result.Status)

		joined := fmt.Sprintf("%v", result.Details)
		assert.Contains(t, joined, "[WARN] No NooBaa core pods found")
	})
}
