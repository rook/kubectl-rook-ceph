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
	"context"
	"fmt"
	"io"
	"time"

	"github.com/rook/kubectl-rook-ceph/pkg/logging"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const logReconnectInterval = 2 * time.Second

// StreamPodLogs copies the logs of the pod matching labelSelector to out. Unlike WaitForPodToRun it
// does not wait for the pod to be running, so logs remain reachable while a pod is crash looping or
// stuck pending.
//
// When opts.Follow is set the stream is re-established after it ends, so following a deployment
// survives a container crash or a rollout onto a replacement pod. This deliberately differs from
// `kubectl logs -f`, which stops as soon as the container it attached to exits.
func StreamPodLogs(ctx context.Context, k8sclientset kubernetes.Interface, namespace, labelSelector string, opts *corev1.PodLogOptions, out io.Writer) error {
	var (
		lastPod  string
		lastRead time.Time
	)

	for attempt := 0; ; attempt++ {
		pod, matches, err := findPodForLogs(ctx, k8sclientset, namespace, labelSelector)
		if err != nil {
			// Fail fast while nothing has been read yet. Once following, a missing pod only means
			// its replacement has not been created yet.
			if attempt == 0 || !opts.Follow {
				return err
			}
			if !waitBeforeReconnect(ctx) {
				return nil
			}
			continue
		}

		if pod.Name != lastPod {
			if matches > 1 {
				logging.Warning("%d pods match label %q in namespace %q, showing logs for pod %q", matches, labelSelector, namespace, pod.Name)
			} else if attempt > 0 {
				logging.Info("following logs from pod %q", pod.Name)
			}
		}

		opened, err := copyPodLogs(ctx, k8sclientset, namespace, pod.Name, reconnectOptions(opts, pod.Name, lastPod, lastRead), out)
		if opened {
			lastRead = time.Now()
			lastPod = pod.Name
		}
		if err != nil {
			if attempt == 0 || !opts.Follow {
				return err
			}
			logging.Warning("%v, reconnecting in %s", err, logReconnectInterval)
		}

		if !opts.Follow || ctx.Err() != nil {
			return nil
		}
		if !waitBeforeReconnect(ctx) {
			return nil
		}
	}
}

func findPodForLogs(ctx context.Context, k8sclientset kubernetes.Interface, namespace, labelSelector string) (*corev1.Pod, int, error) {
	pods, err := k8sclientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list pods matching label %q in namespace %q. %w", labelSelector, namespace, err)
	}

	pod := selectPodForLogs(pods.Items)
	if pod == nil {
		return nil, 0, fmt.Errorf("no pod matching label %q found in namespace %q", labelSelector, namespace)
	}

	return pod, len(pods.Items), nil
}

// copyPodLogs reports whether the stream was opened, so that a caller following the logs can tell a
// stream that ended from one that never started.
func copyPodLogs(ctx context.Context, k8sclientset kubernetes.Interface, namespace, podName string, opts *corev1.PodLogOptions, out io.Writer) (bool, error) {
	stream, err := k8sclientset.CoreV1().Pods(namespace).GetLogs(podName, opts).Stream(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to get logs for pod %q. %w", podName, err)
	}
	defer stream.Close()

	if _, err := io.Copy(out, stream); err != nil {
		return true, fmt.Errorf("failed to stream logs for pod %q. %w", podName, err)
	}

	return true, nil
}

// reconnectOptions adapts the caller's options for a re-established stream. --tail and --previous
// describe the first connection only, and reattaching to a pod already read from must skip what was
// printed before. A replacement pod is read from its beginning, since a rollout can start it before
// the outgoing pod's stream ends.
func reconnectOptions(opts *corev1.PodLogOptions, podName, lastPod string, lastRead time.Time) *corev1.PodLogOptions {
	if lastPod == "" {
		return opts
	}

	next := *opts
	next.TailLines = nil
	next.Previous = false
	if podName == lastPod {
		// A second of overlap costs a few duplicate lines and avoids dropping any to clock skew
		// between this host and the node.
		since := metav1.NewTime(lastRead.Add(-time.Second))
		next.SinceTime = &since
		next.SinceSeconds = nil
	}

	return &next
}

func waitBeforeReconnect(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(logReconnectInterval):
		return true
	}
}

// selectPodForLogs prefers a running pod that is not terminating, so that a rollout leaving both an
// old and a new pod behind reads from the live one. It falls back to the first pod so that logs of a
// pending or crash looping pod are still reachable.
func selectPodForLogs(pods []corev1.Pod) *corev1.Pod {
	if len(pods) == 0 {
		return nil
	}

	for i := range pods {
		if pods[i].Status.Phase == corev1.PodRunning && pods[i].DeletionTimestamp.IsZero() {
			return &pods[i]
		}
	}

	return &pods[0]
}
