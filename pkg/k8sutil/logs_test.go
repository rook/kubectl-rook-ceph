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

package k8sutil

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func testPod(name string, phase corev1.PodPhase, terminating bool) corev1.Pod {
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "rook-ceph",
			Labels:    map[string]string{"app": "rook-ceph-operator"},
		},
		Status: corev1.PodStatus{Phase: phase},
	}
	if terminating {
		now := metav1.NewTime(time.Now())
		pod.DeletionTimestamp = &now
		pod.Finalizers = []string{"test/finalizer"}
	}
	return pod
}

func TestSelectPodForLogs(t *testing.T) {
	tests := []struct {
		name string
		pods []corev1.Pod
		want string
	}{
		{
			name: "no pods",
			pods: nil,
			want: "",
		},
		{
			name: "single running pod",
			pods: []corev1.Pod{testPod("operator-a", corev1.PodRunning, false)},
			want: "operator-a",
		},
		{
			name: "prefers the running pod over a pending one",
			pods: []corev1.Pod{
				testPod("operator-pending", corev1.PodPending, false),
				testPod("operator-running", corev1.PodRunning, false),
			},
			want: "operator-running",
		},
		{
			name: "skips a terminating pod during a rollout",
			pods: []corev1.Pod{
				testPod("operator-old", corev1.PodRunning, true),
				testPod("operator-new", corev1.PodRunning, false),
			},
			want: "operator-new",
		},
		{
			name: "falls back to the first pod when none are running",
			pods: []corev1.Pod{
				testPod("operator-pending", corev1.PodPending, false),
				testPod("operator-failed", corev1.PodFailed, false),
			},
			want: "operator-pending",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pod := selectPodForLogs(tt.pods)
			if tt.want == "" {
				assert.Nil(t, pod)
				return
			}
			require.NotNil(t, pod)
			assert.Equal(t, tt.want, pod.Name)
		})
	}
}

func TestStreamPodLogs(t *testing.T) {
	ctx := context.TODO()

	t.Run("no pod matches the label", func(t *testing.T) {
		client := fake.NewSimpleClientset()
		var out bytes.Buffer

		err := StreamPodLogs(ctx, client, "rook-ceph", "app=rook-ceph-operator", &corev1.PodLogOptions{}, &out)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no pod matching label")
		assert.Empty(t, out.String())
	})

	t.Run("pods in the namespace that do not match the label are ignored", func(t *testing.T) {
		other := testPod("some-other-pod", corev1.PodRunning, false)
		other.Labels = map[string]string{"app": "rook-ceph-tools"}
		client := fake.NewSimpleClientset(&other)
		var out bytes.Buffer

		err := StreamPodLogs(ctx, client, "rook-ceph", "app=rook-ceph-operator", &corev1.PodLogOptions{}, &out)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no pod matching label")
	})

	t.Run("copies the log stream to the writer", func(t *testing.T) {
		pod := testPod("operator-a", corev1.PodRunning, false)
		client := fake.NewSimpleClientset(&pod)
		var out bytes.Buffer

		err := StreamPodLogs(ctx, client, "rook-ceph", "app=rook-ceph-operator", &corev1.PodLogOptions{}, &out)
		require.NoError(t, err)
		assert.Equal(t, "fake logs", out.String())
	})
}

func TestStreamPodLogsFollow(t *testing.T) {
	t.Run("fails fast when there is nothing to follow", func(t *testing.T) {
		client := fake.NewSimpleClientset()
		var out bytes.Buffer

		err := StreamPodLogs(context.TODO(), client, "rook-ceph", "app=rook-ceph-operator", &corev1.PodLogOptions{Follow: true}, &out)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no pod matching label")
	})

	t.Run("stops reconnecting when the context is cancelled", func(t *testing.T) {
		pod := testPod("operator-a", corev1.PodRunning, false)
		client := fake.NewSimpleClientset(&pod)
		var out bytes.Buffer

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		err := StreamPodLogs(ctx, client, "rook-ceph", "app=rook-ceph-operator", &corev1.PodLogOptions{Follow: true}, &out)
		require.NoError(t, err)
		assert.Equal(t, "fake logs", out.String())
	})
}

func TestReconnectOptions(t *testing.T) {
	lastRead := time.Date(2026, 7, 29, 16, 44, 55, 0, time.UTC)
	tail := int64(5)
	sinceSeconds := int64(300)
	original := &corev1.PodLogOptions{
		Container:    "rook-ceph-operator",
		Follow:       true,
		Previous:     true,
		Timestamps:   true,
		TailLines:    &tail,
		SinceSeconds: &sinceSeconds,
	}

	t.Run("the first connection uses the caller's options unchanged", func(t *testing.T) {
		opts := reconnectOptions(original, "operator-a", "", time.Time{})
		assert.Same(t, original, opts)
	})

	t.Run("reattaching to the same pod resumes from the checkpoint", func(t *testing.T) {
		opts := reconnectOptions(original, "operator-a", "operator-a", lastRead)

		require.NotNil(t, opts.SinceTime)
		assert.Equal(t, lastRead.Add(-time.Second), opts.SinceTime.Time)
		// SinceTime and SinceSeconds are mutually exclusive, the apiserver rejects both
		assert.Nil(t, opts.SinceSeconds)
		assert.Nil(t, opts.TailLines)
		assert.False(t, opts.Previous)
		assert.True(t, opts.Follow)
		assert.True(t, opts.Timestamps)
	})

	t.Run("a replacement pod is read from its beginning", func(t *testing.T) {
		opts := reconnectOptions(original, "operator-b", "operator-a", lastRead)

		assert.Nil(t, opts.SinceTime)
		assert.Nil(t, opts.TailLines)
		assert.False(t, opts.Previous)
	})

	t.Run("the caller's options are not mutated", func(t *testing.T) {
		reconnectOptions(original, "operator-b", "operator-a", lastRead)

		assert.Equal(t, &tail, original.TailLines)
		assert.Equal(t, &sinceSeconds, original.SinceSeconds)
		assert.True(t, original.Previous)
		assert.Nil(t, original.SinceTime)
	})
}
