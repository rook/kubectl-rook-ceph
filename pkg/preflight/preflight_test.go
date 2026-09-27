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
	"errors"
	"strings"
	"testing"

	"github.com/rook/kubectl-rook-ceph/pkg/health"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/version"
)

func testNode(name string, mutate ...func(*corev1.Node)) corev1.Node {
	node := corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Status: corev1.NodeStatus{
			Conditions:  []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}},
			NodeInfo:    corev1.NodeSystemInfo{Architecture: "amd64", KernelVersion: "6.8.0-45-generic"},
			Allocatable: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("16Gi")},
		},
	}
	for _, m := range mutate {
		m(&node)
	}
	return node
}

func disk(path string, available bool, reasons ...string) device {
	d := device{Path: path, Available: available, RejectedReasons: reasons}
	d.SysAPI.Size = 20 * 1024 * 1024 * 1024
	d.SysAPI.HumanReadableSize = "20.00 GB"
	return d
}

func probeWithDisks(node string, devices ...device) nodeProbe {
	return nodeProbe{node: node, kernel: "6.8.0-45-generic", rbd: "loaded", devices: devices}
}

func TestParseProbeOutput(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		output := `kernel=6.8.0-45-generic
rbd=available
---inventory---
[{"path": "/dev/vdb", "available": true, "rejected_reasons": [], "sys_api": {"size": 21474836480.0, "human_readable_size": "20.00 GB"}},
 {"path": "/dev/vda", "available": false, "rejected_reasons": ["Has a FileSystem"], "sys_api": {"size": 107374182400.0, "human_readable_size": "100.00 GB"}}]
`
		probe := parseProbeOutput("node-a", output)
		require.NoError(t, probe.err)
		assert.Equal(t, "6.8.0-45-generic", probe.kernel)
		assert.Equal(t, "available", probe.rbd)
		require.Len(t, probe.devices, 2)
		assert.True(t, probe.devices[0].Available)
		assert.Equal(t, []string{"Has a FileSystem"}, probe.devices[1].RejectedReasons)
		assert.Equal(t, "20.00 GB", probe.devices[0].SysAPI.HumanReadableSize)
	})

	t.Run("rbd missing", func(t *testing.T) {
		probe := parseProbeOutput("node-a", "rbd=missing\n---inventory---\n[]")
		require.NoError(t, probe.err)
		assert.Equal(t, rbdMissing, probe.rbd)
	})

	t.Run("no marker", func(t *testing.T) {
		probe := parseProbeOutput("node-a", "sh: ceph-volume: not found")
		assert.ErrorContains(t, probe.err, "unexpected probe output")
	})

	t.Run("bad inventory", func(t *testing.T) {
		probe := parseProbeOutput("node-a", "---inventory---\nTraceback (most recent call last):")
		assert.ErrorContains(t, probe.err, "failed to parse ceph-volume inventory")
	})
}

func TestProbePod(t *testing.T) {
	pod := probePod("rook-ceph", "quay.io/ceph/ceph:v20.2.4", "node-a")
	assert.Equal(t, "node-a", pod.Spec.NodeName)
	assert.Equal(t, corev1.RestartPolicyNever, pod.Spec.RestartPolicy)
	require.NotNil(t, pod.Spec.ActiveDeadlineSeconds)
	require.Len(t, pod.Spec.Containers, 1)
	container := pod.Spec.Containers[0]
	assert.Equal(t, "quay.io/ceph/ceph:v20.2.4", container.Image)
	assert.True(t, *container.SecurityContext.Privileged)
	for _, mount := range container.VolumeMounts {
		if mount.Name != "dev" {
			assert.True(t, mount.ReadOnly, "mount %q should be read-only", mount.Name)
		}
	}
	assert.Contains(t, container.Command[2], "ceph-volume inventory --format json")
}

func TestCheckKubernetesVersion(t *testing.T) {
	assert.Equal(t, health.StatusOK, checkKubernetesVersion(&version.Info{Major: "1", Minor: "34", GitVersion: "v1.34.1"}, nil).Status)
	assert.Equal(t, health.StatusOK, checkKubernetesVersion(&version.Info{Major: "1", Minor: "37+", GitVersion: "v1.37.0-eks"}, nil).Status)
	assert.Equal(t, health.StatusWarning, checkKubernetesVersion(&version.Info{Major: "1", Minor: "29", GitVersion: "v1.29.0"}, nil).Status)
	assert.Equal(t, health.StatusError, checkKubernetesVersion(nil, errors.New("unreachable")).Status)
}

func TestCheckNodeArchitecture(t *testing.T) {
	arm := testNode("b", func(n *corev1.Node) { n.Status.NodeInfo.Architecture = "arm64" })
	assert.Equal(t, health.StatusOK, checkNodeArchitecture([]corev1.Node{testNode("a"), arm}).Status)

	ppc := testNode("c", func(n *corev1.Node) { n.Status.NodeInfo.Architecture = "ppc64le" })
	result := checkNodeArchitecture([]corev1.Node{testNode("a"), ppc})
	assert.Equal(t, health.StatusCritical, result.Status)
	assert.Contains(t, result.Details[0], "c: ppc64le")
}

func TestCheckMonPlacement(t *testing.T) {
	t.Run("enough nodes", func(t *testing.T) {
		result := checkMonPlacement([]corev1.Node{testNode("a"), testNode("b"), testNode("c")})
		assert.Equal(t, health.StatusOK, result.Status)
	})

	t.Run("single node", func(t *testing.T) {
		result := checkMonPlacement([]corev1.Node{testNode("a")})
		assert.Equal(t, health.StatusWarning, result.Status)
		assert.Contains(t, strings.Join(result.Details, " "), "cluster-test.yaml")
	})

	t.Run("no schedulable nodes", func(t *testing.T) {
		cordoned := testNode("a", func(n *corev1.Node) { n.Spec.Unschedulable = true })
		assert.Equal(t, health.StatusCritical, checkMonPlacement([]corev1.Node{cordoned}).Status)
	})

	t.Run("tainted and cordoned nodes are not counted", func(t *testing.T) {
		tainted := testNode("b", func(n *corev1.Node) {
			n.Spec.Taints = []corev1.Taint{{Key: "node-role.kubernetes.io/control-plane", Effect: corev1.TaintEffectNoSchedule}}
		})
		cordoned := testNode("c", func(n *corev1.Node) { n.Spec.Unschedulable = true })
		preferNoSchedule := testNode("d", func(n *corev1.Node) {
			n.Spec.Taints = []corev1.Taint{{Key: "x", Effect: corev1.TaintEffectPreferNoSchedule}}
		})
		result := checkMonPlacement([]corev1.Node{testNode("a"), tainted, cordoned, preferNoSchedule})
		assert.Equal(t, health.StatusWarning, result.Status)
		assert.Contains(t, result.Message, "Only 2 node(s)")
		assert.Contains(t, strings.Join(result.Details, " "), "set mon.count to 1")
	})
}

func TestKernelAtLeast(t *testing.T) {
	assert.True(t, kernelAtLeast("6.8.0-45-generic", 5, 11))
	assert.True(t, kernelAtLeast("5.11.0", 5, 11))
	assert.False(t, kernelAtLeast("5.4.0-216-generic", 5, 11))
	assert.False(t, kernelAtLeast("4.18.0-553.el8_10.x86_64", 5, 11))
	assert.True(t, kernelAtLeast("5.14.0-427.el9_4.x86_64", 5, 11))
	assert.True(t, kernelAtLeast("unknown", 5, 11))
}

func TestCheckKernel(t *testing.T) {
	old := testNode("a", func(n *corev1.Node) { n.Status.NodeInfo.KernelVersion = "5.4.0-216-generic" })
	result := checkKernel([]corev1.Node{old}, nil)
	assert.Equal(t, health.StatusWarning, result.Status)
	assert.Contains(t, result.Details[0], "msgr2")

	nodes := []corev1.Node{testNode("a")}
	assert.Equal(t, health.StatusOK, checkKernel(nodes, nil).Status)

	noRBD := probeWithDisks("a")
	noRBD.rbd = rbdMissing
	result = checkKernel(nodes, []nodeProbe{noRBD})
	assert.Equal(t, health.StatusWarning, result.Status)
	assert.Contains(t, result.Details[0], "rbd kernel module")
}

func TestCheckDisks(t *testing.T) {
	t.Run("three nodes with disks", func(t *testing.T) {
		probes := []nodeProbe{
			probeWithDisks("a", disk("/dev/vdb", true)),
			probeWithDisks("b", disk("/dev/vdb", true)),
			probeWithDisks("c", disk("/dev/vdb", true), disk("/dev/vdc", true)),
		}
		result := checkDisks(probes)
		assert.Equal(t, health.StatusOK, result.Status)
		assert.Equal(t, "4 usable disk(s) on 3 node(s)", result.Message)
	})

	t.Run("single node", func(t *testing.T) {
		result := checkDisks([]nodeProbe{probeWithDisks("a", disk("/dev/vdb", true))})
		assert.Equal(t, health.StatusWarning, result.Status)
		assert.Contains(t, strings.Join(result.Details, " "), "pool-test.yaml")
		assert.Contains(t, strings.Join(result.Details, " "), "set the pool size to 1")
	})

	t.Run("no usable disks explains why", func(t *testing.T) {
		probes := []nodeProbe{probeWithDisks("a", disk("/dev/vda", false, "Has a FileSystem"))}
		result := checkDisks(probes)
		assert.Equal(t, health.StatusCritical, result.Status)
		assert.Contains(t, result.Details[0], "/dev/vda (Has a FileSystem)")
	})

	t.Run("devices without media are hidden", func(t *testing.T) {
		nbd := device{Path: "/dev/nbd0", RejectedReasons: []string{"Insufficient space (<5GB)"}}
		probes := []nodeProbe{probeWithDisks("a", disk("/dev/vdb", true), nbd)}
		result := checkDisks(probes)
		require.Len(t, result.Items, 1)
		assert.Equal(t, "a:/dev/vdb", result.Items[0].Name)
	})

	t.Run("probe failure", func(t *testing.T) {
		probes := []nodeProbe{
			probeWithDisks("a", disk("/dev/vdb", true)),
			{node: "b", err: errors.New("probe pod did not finish (ImagePullBackOff)")},
		}
		result := checkDisks(probes)
		assert.Equal(t, health.StatusError, result.Status)
		assert.Contains(t, result.Details[0], "ImagePullBackOff")
		assert.Contains(t, strings.Join(result.Details, " "), "--skip-node-inspection")
	})

}

func TestCheckMemory(t *testing.T) {
	small := testNode("a", func(n *corev1.Node) {
		n.Status.Allocatable = corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("6Gi")}
	})
	probes := []nodeProbe{probeWithDisks("a", disk("/dev/vdb", true), disk("/dev/vdc", true))}

	result := checkMemory([]corev1.Node{small}, probes)
	assert.Equal(t, health.StatusWarning, result.Status)
	assert.Equal(t, "a: 6.0Gi allocatable, about 9.0Gi needed for 2 OSD(s) and a mon", result.Details[0])

	assert.Equal(t, health.StatusOK, checkMemory([]corev1.Node{testNode("a")}, probes).Status)
}
