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
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

const testNamespace = "rook-ceph"

func testPod(name string, phase corev1.PodPhase, terminating bool, containers ...string) corev1.Pod {
	if len(containers) == 0 {
		containers = []string{"rook-ceph-operator"}
	}

	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
			Labels:    map[string]string{"app": "rook-ceph-operator"},
		},
		Status: corev1.PodStatus{Phase: phase},
	}
	for _, container := range containers {
		pod.Spec.Containers = append(pod.Spec.Containers, corev1.Container{Name: container})
	}
	if terminating {
		now := metav1.NewTime(time.Now())
		pod.DeletionTimestamp = &now
		pod.Finalizers = []string{"test/finalizer"}
	}

	return pod
}

func testStreamOptions(out *bytes.Buffer, podLogOptions *corev1.PodLogOptions) LogStreamOptions {
	return LogStreamOptions{
		Namespace:      testNamespace,
		LabelSelector:  "app=rook-ceph-operator",
		PodLogOptions:  podLogOptions,
		MaxLogRequests: DefaultMaxLogRequests,
		Out:            out,
	}
}

func newTestSupervisor(client kubernetes.Interface, out *bytes.Buffer, opts LogStreamOptions) *supervisor {
	s := &supervisor{
		k8sclientset: client,
		opts:         opts,
		out:          &logWriter{out: out},
		active:       map[streamKey]context.CancelFunc{},
	}
	s.open = s.openPodLogs

	return s
}

// outputLines returns the written lines in a stable order, since concurrent streams may finish in
// any order.
func outputLines(out *bytes.Buffer) []string {
	lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	sort.Strings(lines)

	return lines
}

func Test_containersToRead(t *testing.T) {
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{{Name: "blkdevmapper"}, {Name: "activate"}},
			Containers:     []corev1.Container{{Name: "osd"}, {Name: "log-collector"}},
			EphemeralContainers: []corev1.EphemeralContainer{
				{EphemeralContainerCommon: corev1.EphemeralContainerCommon{Name: "debugger"}},
			},
		},
	}

	t.Run("regular containers by default", func(t *testing.T) {
		assert.Equal(t, []string{"osd", "log-collector"}, containersToRead(pod, "", false))
	})

	t.Run("all containers includes init and ephemeral containers", func(t *testing.T) {
		assert.Equal(t, []string{"osd", "log-collector", "blkdevmapper", "activate", "debugger"}, containersToRead(pod, "", true))
	})

	t.Run("a named init container is reachable", func(t *testing.T) {
		// where a PVC-backed OSD that never started logs the reason
		assert.Equal(t, []string{"activate"}, containersToRead(pod, "activate", false))
	})

	t.Run("a named regular container", func(t *testing.T) {
		assert.Equal(t, []string{"osd"}, containersToRead(pod, "osd", false))
	})

	t.Run("a container the pod does not have", func(t *testing.T) {
		assert.Nil(t, containersToRead(pod, "mon", false))
	})
}

func Test_supervisorDiscover(t *testing.T) {
	ctx := context.TODO()

	t.Run("every regular container of every matching pod", func(t *testing.T) {
		mgr := testPod("rook-ceph-mgr-a", corev1.PodRunning, false, "mgr", "watch-active")
		mon := testPod("rook-ceph-mon-a", corev1.PodRunning, false, "mon")
		client := fake.NewSimpleClientset(&mgr, &mon)
		s := newTestSupervisor(client, &bytes.Buffer{}, testStreamOptions(&bytes.Buffer{}, &corev1.PodLogOptions{}))

		keys, err := s.discover(ctx)
		require.NoError(t, err)
		assert.Equal(t, []streamKey{
			{pod: "rook-ceph-mgr-a", container: "mgr"},
			{pod: "rook-ceph-mgr-a", container: "watch-active"},
			{pod: "rook-ceph-mon-a", container: "mon"},
		}, keys)
	})

	t.Run("a container filter narrows the streams", func(t *testing.T) {
		mgr := testPod("rook-ceph-mgr-a", corev1.PodRunning, false, "mgr", "watch-active")
		client := fake.NewSimpleClientset(&mgr)
		opts := testStreamOptions(&bytes.Buffer{}, &corev1.PodLogOptions{})
		opts.Container = "watch-active"
		s := newTestSupervisor(client, &bytes.Buffer{}, opts)

		keys, err := s.discover(ctx)
		require.NoError(t, err)
		assert.Equal(t, []streamKey{{pod: "rook-ceph-mgr-a", container: "watch-active"}}, keys)
	})

	t.Run("no pod matches the label", func(t *testing.T) {
		s := newTestSupervisor(fake.NewSimpleClientset(), &bytes.Buffer{}, testStreamOptions(&bytes.Buffer{}, &corev1.PodLogOptions{}))

		_, err := s.discover(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no pod matching label")
	})

	t.Run("no pod has the requested container", func(t *testing.T) {
		mon := testPod("rook-ceph-mon-a", corev1.PodRunning, false, "mon")
		client := fake.NewSimpleClientset(&mon)
		opts := testStreamOptions(&bytes.Buffer{}, &corev1.PodLogOptions{})
		opts.Container = "log-collector"
		s := newTestSupervisor(client, &bytes.Buffer{}, opts)

		_, err := s.discover(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `no container "log-collector"`)
	})
}

// Test_supervisorStreamFailureIsIsolated is the regression test for a single unreadable pod taking
// the whole target down with it, which is the case a broken cluster is most likely to produce.
func Test_supervisorStreamFailureIsIsolated(t *testing.T) {
	a := testPod("operator-a", corev1.PodRunning, false)
	b := testPod("operator-b", corev1.PodPending, false)
	c := testPod("operator-c", corev1.PodRunning, false)
	client := fake.NewSimpleClientset(&a, &b, &c)

	var out bytes.Buffer
	s := newTestSupervisor(client, &out, testStreamOptions(&out, &corev1.PodLogOptions{}))
	s.open = func(_ context.Context, podName string, _ *corev1.PodLogOptions) (io.ReadCloser, error) {
		if podName == "operator-b" {
			return nil, fmt.Errorf(`container "rook-ceph-operator" in pod %q is waiting to start: ContainerCreating`, podName)
		}

		return io.NopCloser(strings.NewReader(podName + " logged a line\n")), nil
	}

	keys, err := s.discover(context.TODO())
	require.NoError(t, err)
	require.NoError(t, s.reconcile(context.TODO(), keys, true))
	s.wg.Wait()

	// the readable pods are streamed in full rather than being cut off when their sibling fails
	assert.Equal(t, []string{
		"[operator-a] operator-a logged a line",
		"[operator-c] operator-c logged a line",
	}, outputLines(&out))

	// and the failure is still reported
	err = s.err()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "operator-b")
	assert.NotContains(t, err.Error(), "operator-a")
}

// Test_supervisorStreamFailureIsIsolatedWhileFollowing covers the same isolation on the --follow
// path, which is the one a crash looping cluster actually runs: the unreadable pod is retried rather
// than abandoned, and its siblings keep streaming throughout.
func Test_supervisorStreamFailureIsIsolatedWhileFollowing(t *testing.T) {
	restore := logReconnectInterval
	logReconnectInterval = 10 * time.Millisecond
	defer func() { logReconnectInterval = restore }()

	a := testPod("operator-a", corev1.PodRunning, false)
	b := testPod("operator-b", corev1.PodPending, false)
	client := fake.NewSimpleClientset(&a, &b)

	var out bytes.Buffer
	tail := int64(20)
	opts := testStreamOptions(&out, &corev1.PodLogOptions{Follow: true, Previous: true, TailLines: &tail})
	s := newTestSupervisor(client, &out, opts)

	var (
		mu      sync.Mutex
		retries []corev1.PodLogOptions
	)
	s.open = func(_ context.Context, podName string, podLogOptions *corev1.PodLogOptions) (io.ReadCloser, error) {
		if podName == "operator-b" {
			mu.Lock()
			retries = append(retries, *podLogOptions)
			mu.Unlock()

			return nil, fmt.Errorf("container in pod %q is waiting to start: ContainerCreating", podName)
		}

		return io.NopCloser(strings.NewReader(podName + " logged a line\n")), nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	keys, err := s.discover(ctx)
	require.NoError(t, err)
	require.NoError(t, s.reconcile(ctx, keys, true))
	s.wg.Wait()

	assert.Contains(t, out.String(), "[operator-a] operator-a logged a line")
	// the readable pod keeps being re-read while its sibling keeps failing
	assert.Greater(t, strings.Count(out.String(), "operator-a logged a line"), 1)

	mu.Lock()
	defer mu.Unlock()
	require.Greater(t, len(retries), 1, "expected the unreadable pod to be retried, not abandoned")
	// a stream that has never been read is still making its first connection, so retrying it must
	// not quietly turn --previous into the running container's log
	for i, podLogOptions := range retries {
		assert.True(t, podLogOptions.Previous, "attempt %d dropped --previous", i)
		assert.Equal(t, &tail, podLogOptions.TailLines, "attempt %d dropped --tail", i)
	}
}

func Test_supervisorReconcileCap(t *testing.T) {
	ctx := context.TODO()
	blocked := make(chan struct{})
	defer close(blocked)

	newSupervisor := func(client kubernetes.Interface, max int) *supervisor {
		var out bytes.Buffer
		opts := testStreamOptions(&out, &corev1.PodLogOptions{})
		opts.MaxLogRequests = max
		s := newTestSupervisor(client, &out, opts)
		// hold every stream open so the cap is observed against streams that are still running
		s.open = func(ctx context.Context, _ string, _ *corev1.PodLogOptions) (io.ReadCloser, error) {
			return io.NopCloser(readerUntil(ctx, blocked)), nil
		}

		return s
	}

	t.Run("the first pass refuses to start more streams than the limit", func(t *testing.T) {
		a := testPod("operator-a", corev1.PodRunning, false)
		b := testPod("operator-b", corev1.PodRunning, false)
		c := testPod("operator-c", corev1.PodRunning, false)
		s := newSupervisor(fake.NewSimpleClientset(&a, &b, &c), 2)
		defer s.stop()

		keys, err := s.discover(ctx)
		require.NoError(t, err)

		err = s.reconcile(ctx, keys, true)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "3 log streams exceed the --max-log-requests limit of 2")
		assert.Empty(t, s.active)
	})

	t.Run("a later pass keeps the running streams and skips the newcomers", func(t *testing.T) {
		a := testPod("operator-a", corev1.PodRunning, false)
		b := testPod("operator-b", corev1.PodRunning, false)
		client := fake.NewSimpleClientset(&a, &b)
		s := newSupervisor(client, 2)
		defer s.stop()

		keys, err := s.discover(ctx)
		require.NoError(t, err)
		require.NoError(t, s.reconcile(ctx, keys, true))
		require.Len(t, s.active, 2)

		// a new pod sorting before the running ones must not evict them
		aa := testPod("operator-aa", corev1.PodRunning, false)
		_, err = client.CoreV1().Pods(testNamespace).Create(ctx, &aa, metav1.CreateOptions{})
		require.NoError(t, err)

		keys, err = s.discover(ctx)
		require.NoError(t, err)
		require.NoError(t, s.reconcile(ctx, keys, false))

		assert.Len(t, s.active, 2)
		assert.NotNil(t, s.active[streamKey{pod: "operator-a", container: "rook-ceph-operator"}])
		assert.NotNil(t, s.active[streamKey{pod: "operator-b", container: "rook-ceph-operator"}])
		assert.Nil(t, s.active[streamKey{pod: "operator-aa", container: "rook-ceph-operator"}])
	})

	t.Run("a pod that goes away has its stream stopped", func(t *testing.T) {
		a := testPod("operator-a", corev1.PodRunning, false)
		b := testPod("operator-b", corev1.PodRunning, false)
		client := fake.NewSimpleClientset(&a, &b)
		s := newSupervisor(client, DefaultMaxLogRequests)
		defer s.stop()

		keys, err := s.discover(ctx)
		require.NoError(t, err)
		require.NoError(t, s.reconcile(ctx, keys, true))
		require.Len(t, s.active, 2)

		require.NoError(t, client.CoreV1().Pods(testNamespace).Delete(ctx, "operator-b", metav1.DeleteOptions{}))

		keys, err = s.discover(ctx)
		require.NoError(t, err)
		require.NoError(t, s.reconcile(ctx, keys, false))

		assert.Len(t, s.active, 1)
		assert.Nil(t, s.active[streamKey{pod: "operator-b", container: "rook-ceph-operator"}])
	})
}

// readerUntil blocks until the context is cancelled or done is closed, standing in for a log stream
// that stays open.
func readerUntil(ctx context.Context, done chan struct{}) io.Reader {
	return &blockingReader{ctx: ctx, done: done}
}

type blockingReader struct {
	ctx  context.Context
	done chan struct{}
}

func (r *blockingReader) Read(p []byte) (int, error) {
	select {
	case <-r.ctx.Done():
		return 0, io.EOF
	case <-r.done:
		return 0, io.EOF
	}
}

func TestStreamLogs(t *testing.T) {
	ctx := context.TODO()

	t.Run("no pod matches the label", func(t *testing.T) {
		var out bytes.Buffer

		err := StreamLogs(ctx, fake.NewSimpleClientset(), testStreamOptions(&out, &corev1.PodLogOptions{}))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no pod matching label")
		assert.Empty(t, out.String())
	})

	t.Run("pods in the namespace that do not match the label are ignored", func(t *testing.T) {
		other := testPod("some-other-pod", corev1.PodRunning, false)
		other.Labels = map[string]string{"app": "rook-ceph-tools"}
		client := fake.NewSimpleClientset(&other)
		var out bytes.Buffer

		err := StreamLogs(ctx, client, testStreamOptions(&out, &corev1.PodLogOptions{}))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no pod matching label")
	})

	t.Run("a single stream is copied verbatim", func(t *testing.T) {
		pod := testPod("operator-a", corev1.PodRunning, false)
		client := fake.NewSimpleClientset(&pod)
		var out bytes.Buffer

		err := StreamLogs(ctx, client, testStreamOptions(&out, &corev1.PodLogOptions{}))
		require.NoError(t, err)
		assert.Equal(t, "fake logs", out.String())
	})

	t.Run("several pods are prefixed with the pod name", func(t *testing.T) {
		a := testPod("operator-a", corev1.PodRunning, false)
		b := testPod("operator-b", corev1.PodRunning, false)
		client := fake.NewSimpleClientset(&a, &b)
		var out bytes.Buffer

		err := StreamLogs(ctx, client, testStreamOptions(&out, &corev1.PodLogOptions{}))
		require.NoError(t, err)
		assert.Equal(t, []string{"[operator-a] fake logs", "[operator-b] fake logs"}, outputLines(&out))
	})

	t.Run("several containers of one pod are prefixed with the container too", func(t *testing.T) {
		mgr := testPod("rook-ceph-mgr-a", corev1.PodRunning, false, "mgr", "watch-active")
		client := fake.NewSimpleClientset(&mgr)
		var out bytes.Buffer

		err := StreamLogs(ctx, client, testStreamOptions(&out, &corev1.PodLogOptions{}))
		require.NoError(t, err)
		assert.Equal(t, []string{
			"[rook-ceph-mgr-a/mgr] fake logs",
			"[rook-ceph-mgr-a/watch-active] fake logs",
		}, outputLines(&out))
	})

	t.Run("a container filter leaves a single unprefixed stream", func(t *testing.T) {
		mgr := testPod("rook-ceph-mgr-a", corev1.PodRunning, false, "mgr", "watch-active")
		client := fake.NewSimpleClientset(&mgr)
		var out bytes.Buffer
		opts := testStreamOptions(&out, &corev1.PodLogOptions{})
		opts.Container = "mgr"

		err := StreamLogs(ctx, client, opts)
		require.NoError(t, err)
		assert.Equal(t, "fake logs", out.String())
	})

	t.Run("more streams than the limit allows", func(t *testing.T) {
		a := testPod("operator-a", corev1.PodRunning, false)
		b := testPod("operator-b", corev1.PodRunning, false)
		c := testPod("operator-c", corev1.PodRunning, false)
		client := fake.NewSimpleClientset(&a, &b, &c)
		var out bytes.Buffer
		opts := testStreamOptions(&out, &corev1.PodLogOptions{})
		opts.MaxLogRequests = 2

		err := StreamLogs(ctx, client, opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "3 log streams exceed the --max-log-requests limit of 2")
		assert.Empty(t, out.String())
	})

	t.Run("missing options are defaulted rather than panicking", func(t *testing.T) {
		pod := testPod("operator-a", corev1.PodRunning, false)
		client := fake.NewSimpleClientset(&pod)

		err := StreamLogs(ctx, client, LogStreamOptions{
			Namespace:     testNamespace,
			LabelSelector: "app=rook-ceph-operator",
		})
		assert.NoError(t, err)
	})
}

func TestStreamLogsFollow(t *testing.T) {
	restore := logReconnectInterval
	logReconnectInterval = 10 * time.Millisecond
	defer func() { logReconnectInterval = restore }()

	t.Run("fails fast when there is nothing to follow", func(t *testing.T) {
		client := fake.NewSimpleClientset()
		var out bytes.Buffer

		err := StreamLogs(context.TODO(), client, testStreamOptions(&out, &corev1.PodLogOptions{Follow: true}))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no pod matching label")
	})

	t.Run("stops reconnecting when the context is cancelled", func(t *testing.T) {
		pod := testPod("operator-a", corev1.PodRunning, false)
		client := fake.NewSimpleClientset(&pod)
		var out bytes.Buffer

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		err := StreamLogs(ctx, client, testStreamOptions(&out, &corev1.PodLogOptions{Follow: true}))
		require.NoError(t, err)
		assert.Contains(t, out.String(), "fake logs")
	})

	t.Run("a pod that appears later is followed too", func(t *testing.T) {
		a := testPod("operator-a", corev1.PodRunning, false)
		client := fake.NewSimpleClientset(&a)
		var out bytes.Buffer

		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		go func() {
			time.Sleep(50 * time.Millisecond)
			b := testPod("operator-b", corev1.PodRunning, false)
			_, err := client.CoreV1().Pods(testNamespace).Create(context.Background(), &b, metav1.CreateOptions{})
			assert.NoError(t, err)
		}()

		err := StreamLogs(ctx, client, testStreamOptions(&out, &corev1.PodLogOptions{Follow: true}))
		require.NoError(t, err)
		assert.Contains(t, out.String(), "[operator-b] fake logs")
	})
}

func Test_streamOptions(t *testing.T) {
	lastRead := time.Date(2026, 7, 29, 16, 44, 55, 0, time.UTC)
	tail := int64(5)
	sinceSeconds := int64(300)
	original := &corev1.PodLogOptions{
		Follow:       true,
		Previous:     true,
		Timestamps:   true,
		TailLines:    &tail,
		SinceSeconds: &sinceSeconds,
	}

	t.Run("the first connection to a pod that was already there keeps the caller's limits", func(t *testing.T) {
		opts := streamOptions(original, "mon", true, time.Time{})

		assert.Equal(t, "mon", opts.Container)
		assert.Equal(t, &tail, opts.TailLines)
		assert.True(t, opts.Previous)
		assert.Nil(t, opts.SinceTime)
	})

	t.Run("a pod that appeared later is read from its beginning", func(t *testing.T) {
		opts := streamOptions(original, "mon", false, time.Time{})

		assert.Nil(t, opts.TailLines)
		assert.False(t, opts.Previous)
		assert.Nil(t, opts.SinceTime)
		assert.True(t, opts.Follow)
	})

	t.Run("reattaching to a stream resumes from the checkpoint", func(t *testing.T) {
		opts := streamOptions(original, "mon", true, lastRead)

		require.NotNil(t, opts.SinceTime)
		assert.Equal(t, lastRead.Add(-time.Second), opts.SinceTime.Time)
		// SinceTime and SinceSeconds are mutually exclusive, the apiserver rejects both
		assert.Nil(t, opts.SinceSeconds)
		assert.Nil(t, opts.TailLines)
		assert.False(t, opts.Previous)
		assert.True(t, opts.Timestamps)
	})

	t.Run("retrying a stream that never opened is still its first connection", func(t *testing.T) {
		opts := streamOptions(original, "mon", true, time.Time{})

		assert.Nil(t, opts.SinceTime)
		// dropping these would silently read the running container where --previous was asked for,
		// and the whole log where --tail was
		assert.Equal(t, &tail, opts.TailLines)
		assert.True(t, opts.Previous)
	})

	t.Run("the caller's options are not mutated", func(t *testing.T) {
		streamOptions(original, "mon", true, lastRead)

		assert.Equal(t, &tail, original.TailLines)
		assert.Equal(t, &sinceSeconds, original.SinceSeconds)
		assert.True(t, original.Previous)
		assert.Nil(t, original.SinceTime)
		assert.Empty(t, original.Container)
	})
}

func Test_sharesPod(t *testing.T) {
	tests := []struct {
		name string
		keys []streamKey
		want bool
	}{
		{name: "no streams"},
		{name: "one stream", keys: []streamKey{{pod: "a", container: "mon"}}},
		{
			name: "one container each",
			keys: []streamKey{{pod: "a", container: "mon"}, {pod: "b", container: "mon"}},
		},
		{
			name: "two containers of one pod",
			keys: []streamKey{{pod: "a", container: "mgr"}, {pod: "a", container: "watch-active"}},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, sharesPod(slices.Clone(tt.keys)))
		})
	}
}

func Test_streamWriter(t *testing.T) {
	key := streamKey{pod: "operator-a", container: "rook-ceph-operator"}

	t.Run("a line split across writes is emitted once", func(t *testing.T) {
		var out bytes.Buffer
		parent := &logWriter{out: &out}
		parent.enablePrefix(false)
		w := parent.stream(key)

		_, err := w.Write([]byte("first "))
		require.NoError(t, err)
		_, err = w.Write([]byte("half\nsecond line\n"))
		require.NoError(t, err)

		assert.Equal(t, "[operator-a] first half\n[operator-a] second line\n", out.String())
	})

	t.Run("an unterminated final line is flushed", func(t *testing.T) {
		var out bytes.Buffer
		parent := &logWriter{out: &out}
		parent.enablePrefix(true)
		w := parent.stream(key)

		_, err := w.Write([]byte("no trailing newline"))
		require.NoError(t, err)
		assert.Empty(t, out.String())

		w.flush()
		assert.Equal(t, "[operator-a/rook-ceph-operator] no trailing newline\n", out.String())
	})

	t.Run("without a prefix the bytes are unchanged", func(t *testing.T) {
		var out bytes.Buffer
		w := (&logWriter{out: &out}).stream(key)

		_, err := w.Write([]byte("a line\nand a partial"))
		require.NoError(t, err)
		w.flush()

		assert.Equal(t, "a line\nand a partial", out.String())
	})
}
