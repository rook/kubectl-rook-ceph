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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func testNode(name string, conditions []v1.NodeCondition) *v1.Node {
	return &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Status: v1.NodeStatus{
			Conditions: conditions,
		},
	}
}

func readyCondition() v1.NodeCondition {
	return v1.NodeCondition{Type: v1.NodeReady, Status: v1.ConditionTrue}
}

func TestGetNodePressures(t *testing.T) {
	t.Run("no pressure", func(t *testing.T) {
		node := testNode("node1", []v1.NodeCondition{
			readyCondition(),
			{Type: v1.NodeMemoryPressure, Status: v1.ConditionFalse},
			{Type: v1.NodeDiskPressure, Status: v1.ConditionFalse},
			{Type: v1.NodePIDPressure, Status: v1.ConditionFalse},
		})
		assert.Empty(t, getNodePressures(node))
	})

	t.Run("memory pressure", func(t *testing.T) {
		node := testNode("node1", []v1.NodeCondition{
			{Type: v1.NodeMemoryPressure, Status: v1.ConditionTrue},
			{Type: v1.NodeDiskPressure, Status: v1.ConditionFalse},
		})
		pressures := getNodePressures(node)
		assert.Equal(t, []string{"MemoryPressure"}, pressures)
	})

	t.Run("multiple pressures", func(t *testing.T) {
		node := testNode("node1", []v1.NodeCondition{
			{Type: v1.NodeMemoryPressure, Status: v1.ConditionTrue},
			{Type: v1.NodeDiskPressure, Status: v1.ConditionTrue},
			{Type: v1.NodePIDPressure, Status: v1.ConditionFalse},
		})
		pressures := getNodePressures(node)
		assert.Equal(t, []string{"MemoryPressure", "DiskPressure"}, pressures)
	})
}

func TestIsNodeReady(t *testing.T) {
	t.Run("ready", func(t *testing.T) {
		node := testNode("node1", []v1.NodeCondition{readyCondition()})
		assert.True(t, isNodeReady(node))
	})

	t.Run("not ready", func(t *testing.T) {
		node := testNode("node1", []v1.NodeCondition{
			{Type: v1.NodeReady, Status: v1.ConditionFalse},
		})
		assert.False(t, isNodeReady(node))
	})

	t.Run("no ready condition", func(t *testing.T) {
		node := testNode("node1", []v1.NodeCondition{})
		assert.False(t, isNodeReady(node))
	})
}

func TestCheckNodeResourcePressure(t *testing.T) {
	t.Run("all healthy", func(t *testing.T) {
		client := fake.NewSimpleClientset(
			testNode("node1", []v1.NodeCondition{
				readyCondition(),
				{Type: v1.NodeMemoryPressure, Status: v1.ConditionFalse},
				{Type: v1.NodeDiskPressure, Status: v1.ConditionFalse},
				{Type: v1.NodePIDPressure, Status: v1.ConditionFalse},
			}),
			testNode("node2", []v1.NodeCondition{
				readyCondition(),
				{Type: v1.NodeMemoryPressure, Status: v1.ConditionFalse},
				{Type: v1.NodeDiskPressure, Status: v1.ConditionFalse},
				{Type: v1.NodePIDPressure, Status: v1.ConditionFalse},
			}),
		)

		result := checkNodeResourcePressure(context.Background(), client, "")
		assert.Equal(t, StatusOK, result.Status)
		assert.Contains(t, result.Message, "2 nodes are healthy")
		require.Len(t, result.Details, 1)
		assert.Contains(t, result.Details[0], "[INFO]")
		assert.Contains(t, result.Details[0], "2 nodes checked")
		assert.Len(t, result.Items, 2, "verbose should list all nodes")
		assert.Contains(t, result.Items[0].Details, "Ready=True")
	})

	t.Run("one node with memory pressure", func(t *testing.T) {
		client := fake.NewSimpleClientset(
			testNode("node1", []v1.NodeCondition{
				readyCondition(),
				{Type: v1.NodeMemoryPressure, Status: v1.ConditionTrue},
				{Type: v1.NodeDiskPressure, Status: v1.ConditionFalse},
			}),
			testNode("node2", []v1.NodeCondition{
				readyCondition(),
				{Type: v1.NodeMemoryPressure, Status: v1.ConditionFalse},
				{Type: v1.NodeDiskPressure, Status: v1.ConditionFalse},
			}),
		)

		result := checkNodeResourcePressure(context.Background(), client, "")
		assert.Equal(t, StatusWarning, result.Status)
		assert.Contains(t, result.Message, "1 node(s) under resource pressure")

		require.Len(t, result.Details, 1)
		assert.Contains(t, result.Details[0], "[WARN]")
		assert.Contains(t, result.Details[0], "node1")
		assert.Contains(t, result.Details[0], "MemoryPressure")

		assert.Len(t, result.Items, 2, "verbose should list all nodes")
	})

	t.Run("node not ready is critical", func(t *testing.T) {
		client := fake.NewSimpleClientset(
			testNode("node1", []v1.NodeCondition{
				{Type: v1.NodeReady, Status: v1.ConditionFalse},
				{Type: v1.NodeMemoryPressure, Status: v1.ConditionFalse},
			}),
		)

		result := checkNodeResourcePressure(context.Background(), client, "")
		assert.Equal(t, StatusCritical, result.Status)
		assert.Contains(t, result.Message, "not ready")

		require.Len(t, result.Details, 1)
		assert.Contains(t, result.Details[0], "[WARN]")
		assert.Contains(t, result.Details[0], "NotReady")

		require.Len(t, result.Items, 1)
		assert.Equal(t, "NotReady", result.Items[0].Status)
		assert.Contains(t, result.Items[0].Details, "Ready=False")
	})

	t.Run("pressure and not ready", func(t *testing.T) {
		client := fake.NewSimpleClientset(
			testNode("node1", []v1.NodeCondition{
				{Type: v1.NodeReady, Status: v1.ConditionFalse},
				{Type: v1.NodeDiskPressure, Status: v1.ConditionTrue},
			}),
		)

		result := checkNodeResourcePressure(context.Background(), client, "")
		assert.Equal(t, StatusCritical, result.Status)

		require.Len(t, result.Details, 1)
		assert.Contains(t, result.Details[0], "NotReady")
		assert.Contains(t, result.Details[0], "DiskPressure")

		require.Len(t, result.Items, 1)
		assert.Contains(t, result.Items[0].Details, "Ready=False")
		assert.Contains(t, result.Items[0].Details, "DiskPressure=True")
	})

	t.Run("no storage nodes", func(t *testing.T) {
		client := fake.NewSimpleClientset()

		result := checkNodeResourcePressure(context.Background(), client, "")
		assert.Equal(t, StatusOK, result.Status)
		assert.Contains(t, result.Message, "No nodes found")
	})
}
