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

package preflight

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/ptr"
)

const (
	probeTimeout    = 3 * time.Minute
	inventoryMarker = "---inventory---"
	// rbdMissing is the probe's rbd value when the module is neither loaded, built in, nor
	// installed. The other values are "loaded", "builtin" and "available".
	rbdMissing = "missing"
)

// probeScript reports the kernel, whether the rbd module can be used, and the output of
// "ceph-volume inventory". The OSD prepare job uses the same
// inventory to decide which devices it can use, so the results match what Rook would do. Module
// directories are checked in the usual location and the NixOS location.
const probeScript = `
kernel=$(uname -r)
echo "kernel=${kernel}"
rbd=missing
if [ -d /sys/module/rbd ]; then
  rbd=loaded
else
  for dir in /host/lib/modules /host/usr/lib/modules /host/run/current-system/kernel-modules/lib/modules; do
    if grep -qs '/rbd\.ko' "${dir}/${kernel}/modules.builtin"; then rbd=builtin; break; fi
    if grep -qs '/rbd\.ko' "${dir}/${kernel}/modules.dep"; then rbd=available; break; fi
  done
fi
echo "rbd=${rbd}"
echo "` + inventoryMarker + `"
ceph-volume inventory --format json
`

// nodeProbe is what the probe pod found on one node.
type nodeProbe struct {
	node    string
	err     error
	kernel  string
	rbd     string
	devices []device
}

// device is one entry of "ceph-volume inventory --format json".
type device struct {
	Path            string   `json:"path"`
	Available       bool     `json:"available"`
	RejectedReasons []string `json:"rejected_reasons"`
	SysAPI          struct {
		Size              float64 `json:"size"`
		HumanReadableSize string  `json:"human_readable_size"`
	} `json:"sys_api"`
}

func parseProbeOutput(node, output string) nodeProbe {
	probe := nodeProbe{node: node}
	header, inventory, found := strings.Cut(output, inventoryMarker)
	if !found {
		probe.err = fmt.Errorf("unexpected probe output: %q", truncate(output, 200))
		return probe
	}
	for _, line := range strings.Split(header, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		switch key {
		case "kernel":
			probe.kernel = value
		case "rbd":
			probe.rbd = value
		}
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(inventory)), &probe.devices); err != nil {
		probe.err = fmt.Errorf("failed to parse ceph-volume inventory: %v", err)
	}
	return probe
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// probeNodes runs a probe pod on each node in parallel and returns the results in the order of
// the nodes. The pods are deleted before it returns.
func probeNodes(ctx context.Context, k8sclientset kubernetes.Interface, namespace, image string, nodes []string) []nodeProbe {
	results := make([]nodeProbe, len(nodes))
	var wg sync.WaitGroup
	for i, node := range nodes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = probeNode(ctx, k8sclientset, namespace, image, node)
		}()
	}
	wg.Wait()
	return results
}

func probeNode(ctx context.Context, k8sclientset kubernetes.Interface, namespace, image, node string) nodeProbe {
	pod, err := k8sclientset.CoreV1().Pods(namespace).Create(ctx, probePod(namespace, image, node), metav1.CreateOptions{})
	if err != nil {
		return nodeProbe{node: node, err: fmt.Errorf("failed to create probe pod: %v", err)}
	}
	defer func() {
		// Clean up even if ctx was cancelled.
		deleteCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = k8sclientset.CoreV1().Pods(namespace).Delete(deleteCtx, pod.Name, metav1.DeleteOptions{
			GracePeriodSeconds: ptr.To[int64](0),
		})
	}()

	var phase corev1.PodPhase
	var waitingReason string
	err = wait.PollUntilContextTimeout(ctx, 2*time.Second, probeTimeout, true, func(ctx context.Context) (bool, error) {
		p, err := k8sclientset.CoreV1().Pods(namespace).Get(ctx, pod.Name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		phase = p.Status.Phase
		for _, status := range p.Status.ContainerStatuses {
			if status.State.Waiting != nil {
				waitingReason = status.State.Waiting.Reason
			}
		}
		return phase == corev1.PodSucceeded || phase == corev1.PodFailed, nil
	})
	if err != nil {
		if waitingReason != "" {
			return nodeProbe{node: node, err: fmt.Errorf("probe pod did not finish (%s)", waitingReason)}
		}
		return nodeProbe{node: node, err: fmt.Errorf("probe pod did not finish (phase %s): %v", phase, err)}
	}

	logs, err := k8sclientset.CoreV1().Pods(namespace).GetLogs(pod.Name, &corev1.PodLogOptions{}).DoRaw(ctx)
	if err != nil {
		return nodeProbe{node: node, err: fmt.Errorf("failed to read probe pod logs: %v", err)}
	}
	if phase == corev1.PodFailed {
		return nodeProbe{node: node, err: fmt.Errorf("probe pod failed: %s", truncate(strings.TrimSpace(string(logs)), 300))}
	}
	return parseProbeOutput(node, string(logs))
}

func probePod(namespace, image, node string) *corev1.Pod {
	hostPathVolume := func(name, path string) corev1.Volume {
		return corev1.Volume{Name: name, VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: path}}}
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "rook-ceph-preflight-",
			Namespace:    namespace,
			Labels:       map[string]string{"app": "rook-ceph-preflight"},
		},
		Spec: corev1.PodSpec{
			// Setting the node name skips the scheduler, so the probe also runs on nodes that are
			// cordoned or tainted. The toleration keeps it from being evicted by NoExecute taints.
			NodeName:              node,
			RestartPolicy:         corev1.RestartPolicyNever,
			ActiveDeadlineSeconds: ptr.To(int64(probeTimeout.Seconds())),
			Tolerations:           []corev1.Toleration{{Operator: corev1.TolerationOpExists}},
			Containers: []corev1.Container{{
				Name:    "probe",
				Image:   image,
				Command: []string{"/bin/sh", "-c", probeScript},
				// Reading block devices and their signatures needs a privileged container, as it
				// does for the OSD prepare job.
				SecurityContext: &corev1.SecurityContext{Privileged: ptr.To(true)},
				VolumeMounts: []corev1.VolumeMount{
					{Name: "dev", MountPath: "/dev"},
					{Name: "sys", MountPath: "/sys", ReadOnly: true},
					{Name: "udev", MountPath: "/run/udev", ReadOnly: true},
					{Name: "host", MountPath: "/host", ReadOnly: true},
				},
			}},
			Volumes: []corev1.Volume{
				hostPathVolume("dev", "/dev"),
				hostPathVolume("sys", "/sys"),
				hostPathVolume("udev", "/run/udev"),
				hostPathVolume("host", "/"),
			},
		},
	}
}
