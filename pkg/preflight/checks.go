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
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/rook/kubectl-rook-ceph/pkg/health"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/version"
)

const (
	CheckKubernetesVersion = "Kubernetes Version"
	CheckNodeArchitecture  = "Node Architecture"
	CheckMonPlacement      = "Mon Placement"
	CheckKernel            = "Kernel"
	CheckDisks             = "Disks for OSDs"
	CheckMemory            = "Node Memory"

	prerequisitesURL = "https://rook.io/docs/rook/latest/Getting-Started/Prerequisites/prerequisites/"
	// Used when the minimum cannot be read from prerequisitesURL, for example without internet access.
	fallbackKubernetesMinor = 32

	// Rook connects with msgr2 by default (rook/rook#18090), which the kernel RBD and CephFS
	// clients support from kernel 5.11.
	msgr2KernelMajor = 5
	msgr2KernelMinor = 11

	// Rook's default mon count. Each mon runs on its own node.
	defaultMonCount = 3

	// Ceph's default of 3 replicas with host failure domain needs OSDs on 3 nodes.
	defaultReplicaHosts = 3

	inspectionHint = "The node inspection pods need permission to run privileged pods. Use --probe-namespace to run them in another namespace, or --skip-node-inspection to skip the checks that need them."
)

// Rough memory needs of the Ceph daemons with default settings, used to warn about nodes that are
// clearly too small. osd_memory_target defaults to 4GiB.
var (
	osdMemory = resource.MustParse("4Gi")
	monMemory = resource.MustParse("1Gi")
)

var supportedArchitectures = map[string]bool{"amd64": true, "arm64": true}

// Matches "Kubernetes versions <strong>v1.32</strong> through ..." on the prerequisites page.
var minKubernetesRegexp = regexp.MustCompile(`Kubernetes versions\s*(?:<[^>]*>\s*)*v1\.(\d+)`)

// minKubernetesMinor reads the minimum Kubernetes minor version from Rook's prerequisites page,
// so it follows new Rook releases without a plugin update.
func minKubernetesMinor(ctx context.Context) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, prerequisitesURL, nil)
	if err != nil {
		return 0, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("%s returned %s", prerequisitesURL, resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return 0, err
	}
	return parseMinKubernetesMinor(string(body))
}

func parseMinKubernetesMinor(page string) (int, error) {
	match := minKubernetesRegexp.FindStringSubmatch(page)
	if match == nil {
		return 0, fmt.Errorf("no Kubernetes version found on %s", prerequisitesURL)
	}
	return strconv.Atoi(match[1])
}

// checkKubernetesVersion warns if the server is older than minMinor. There is no upper bound, since
// a new Kubernetes release usually works before Rook officially supports it.
func checkKubernetesVersion(info *version.Info, err error, minMinor int) health.CheckResult {
	result := health.CheckResult{Name: CheckKubernetesVersion, Category: health.CategoryK8sResources}
	if err != nil {
		result.Status = health.StatusError
		result.Message = fmt.Sprintf("Failed to get the Kubernetes version: %v", err)
		return result
	}
	major, _ := strconv.Atoi(strings.TrimSuffix(info.Major, "+"))
	minor, _ := strconv.Atoi(strings.TrimSuffix(info.Minor, "+"))
	result.Message = fmt.Sprintf("Kubernetes %s", info.GitVersion)
	if major < 1 || (major == 1 && minor < minMinor) {
		result.Status = health.StatusWarning
		result.Message = fmt.Sprintf("Kubernetes %s is older than v1.%d, the minimum that Rook supports",
			info.GitVersion, minMinor)
		return result
	}
	result.Status = health.StatusOK
	return result
}

func checkNodeArchitecture(nodes []corev1.Node) health.CheckResult {
	result := health.CheckResult{Name: CheckNodeArchitecture, Category: health.CategoryK8sResources}
	for _, node := range nodes {
		arch := node.Status.NodeInfo.Architecture
		result.Items = append(result.Items, health.CheckItem{Name: node.Name, Status: arch})
		if !supportedArchitectures[arch] {
			result.Details = append(result.Details, fmt.Sprintf("%s: %s is not supported", node.Name, arch))
		}
	}
	if len(result.Details) > 0 {
		result.Status = health.StatusCritical
		result.Message = "Some nodes have an architecture that Ceph images are not built for (amd64 and arm64 are supported)"
		return result
	}
	result.Status = health.StatusOK
	result.Message = fmt.Sprintf("All %d nodes are amd64 or arm64", len(nodes))
	return result
}

// schedulableNodes returns the nodes that are ready, not cordoned and not tainted to keep pods off.
func schedulableNodes(nodes []corev1.Node) []corev1.Node {
	var schedulable []corev1.Node
	for _, node := range nodes {
		if node.Spec.Unschedulable || !isNodeReady(&node) || hasBlockingTaint(&node) {
			continue
		}
		schedulable = append(schedulable, node)
	}
	return schedulable
}

func isNodeReady(node *corev1.Node) bool {
	for _, cond := range node.Status.Conditions {
		if cond.Type == corev1.NodeReady {
			return cond.Status == corev1.ConditionTrue
		}
	}
	return false
}

func hasBlockingTaint(node *corev1.Node) bool {
	for _, taint := range node.Spec.Taints {
		if taint.Effect == corev1.TaintEffectNoSchedule || taint.Effect == corev1.TaintEffectNoExecute {
			return true
		}
	}
	return false
}

func checkMonPlacement(nodes []corev1.Node) health.CheckResult {
	result := health.CheckResult{Name: CheckMonPlacement, Category: health.CategoryK8sResources}
	schedulable := schedulableNodes(nodes)
	for _, node := range schedulable {
		result.Items = append(result.Items, health.CheckItem{Name: node.Name, Status: "schedulable"})
	}

	if len(schedulable) >= defaultMonCount {
		result.Status = health.StatusOK
		result.Message = fmt.Sprintf("%d schedulable node(s), enough for %d mons on different nodes", len(schedulable), defaultMonCount)
		return result
	}

	notCounted := "Nodes that are not ready, cordoned, or have a NoSchedule or NoExecute taint are not counted. Rook placement settings can tolerate taints."
	if len(schedulable) == 0 {
		result.Status = health.StatusCritical
		result.Message = "No schedulable nodes, so no mons can run"
		result.Details = append(result.Details, notCounted)
		return result
	}

	result.Status = health.StatusWarning
	result.Message = fmt.Sprintf("Only %d node(s) are schedulable; Rook's default of %d mons needs %d nodes",
		len(schedulable), defaultMonCount, defaultMonCount)
	result.Details = append(result.Details, notCounted)
	if len(schedulable) == 1 {
		result.Details = append(result.Details,
			"For a single-node test cluster, set mon.count to 1 and mon.allowMultiplePerNode, as in deploy/examples/cluster-test.yaml in the Rook repository.")
	} else {
		result.Details = append(result.Details,
			fmt.Sprintf("Add nodes, or set mon.count to %d (an odd number keeps quorum).", largestOdd(len(schedulable))))
	}
	return result
}

func largestOdd(n int) int {
	if n <= 1 {
		return 1
	}
	if n%2 == 0 {
		return n - 1
	}
	return n
}

// checkKernel checks the kernel version for msgr2 and, if the nodes were probed, the rbd module.
func checkKernel(nodes []corev1.Node, probes []nodeProbe) health.CheckResult {
	result := health.CheckResult{Name: CheckKernel, Category: health.CategoryStorage}
	status := health.StatusOK

	for _, node := range nodes {
		kernel := node.Status.NodeInfo.KernelVersion
		result.Items = append(result.Items, health.CheckItem{Name: node.Name, Status: kernel})
		if !kernelAtLeast(kernel, msgr2KernelMajor, msgr2KernelMinor) {
			status = health.StatusWarning
			result.Details = append(result.Details, fmt.Sprintf(
				"%s: kernel %s is older than %d.%d, so RBD and CephFS volumes cannot be mounted with msgr2, the default",
				node.Name, kernel, msgr2KernelMajor, msgr2KernelMinor))
		}
	}

	inspectFailed := false
	for _, probe := range probes {
		if probe.err != nil {
			inspectFailed = true
			result.Details = append(result.Details, fmt.Sprintf("%s: rbd module not checked: %v", probe.node, probe.err))
			continue
		}
		if probe.rbd == rbdMissing {
			status = health.StatusWarning
			result.Details = append(result.Details, fmt.Sprintf(
				"%s: the rbd kernel module was not found; block (RBD) PVCs cannot be mounted with the kernel client", probe.node))
		}
	}

	result.Status = status
	if inspectFailed {
		result.Status = health.StatusError
		result.Message = "Some nodes could not be inspected"
		result.Details = append(result.Details, inspectionHint)
		return result
	}
	if status == health.StatusOK {
		if len(probes) == 0 {
			result.Message = "Kernel versions support msgr2 (the rbd module was not checked)"
		} else {
			result.Message = "Kernel versions support msgr2 and the rbd module is available"
		}
	} else {
		result.Message = "Some nodes have kernel limitations"
	}
	return result
}

// kernelAtLeast compares the major and minor version of a kernel release such as 6.8.0-45-generic.
// An unparsable version is treated as new enough.
func kernelAtLeast(release string, major, minor int) bool {
	parts := strings.SplitN(release, ".", 3)
	if len(parts) < 2 {
		return true
	}
	gotMajor, err1 := strconv.Atoi(parts[0])
	gotMinor, err2 := strconv.Atoi(strings.TrimFunc(parts[1], func(r rune) bool { return r < '0' || r > '9' }))
	if err1 != nil || err2 != nil {
		return true
	}
	return gotMajor > major || (gotMajor == major && gotMinor >= minor)
}

// usableDevices returns, for each probed node, the devices that Rook could turn into OSDs.
func usableDevices(probes []nodeProbe) map[string][]device {
	usable := map[string][]device{}
	for _, probe := range probes {
		for _, dev := range probe.devices {
			if dev.Available {
				usable[probe.node] = append(usable[probe.node], dev)
			}
		}
	}
	return usable
}

func checkDisks(probes []nodeProbe) health.CheckResult {
	result := health.CheckResult{Name: CheckDisks, Category: health.CategoryStorage}
	usable := usableDevices(probes)

	nodesWithDisks, totalDisks := 0, 0
	for _, probe := range probes {
		if probe.err != nil {
			result.Status = health.StatusError
			result.Details = append(result.Details, fmt.Sprintf("%s: %v", probe.node, probe.err))
			continue
		}
		if len(usable[probe.node]) > 0 {
			nodesWithDisks++
			totalDisks += len(usable[probe.node])
		}
		for _, dev := range probe.devices {
			// Devices without media, such as unused nbd devices, are noise.
			if dev.SysAPI.Size == 0 {
				continue
			}
			result.Items = append(result.Items, health.CheckItem{
				Name:    probe.node + ":" + dev.Path,
				Node:    probe.node,
				Status:  dev.SysAPI.HumanReadableSize,
				Details: fmt.Sprintf("%s, %s", dev.SysAPI.HumanReadableSize, deviceDetails(dev)),
			})
		}
		if len(usable[probe.node]) == 0 {
			result.Details = append(result.Details, fmt.Sprintf("%s: no usable disks%s", probe.node, rejectedSummary(probe.devices)))
		}
	}

	result.Message = fmt.Sprintf("%d usable disk(s) on %d node(s)", totalDisks, nodesWithDisks)
	switch {
	case result.Status == health.StatusError:
		result.Details = append(result.Details, inspectionHint)
	case totalDisks == 0:
		result.Status = health.StatusCritical
		result.Message = "No disks that Rook can use were found, so no OSDs can be created"
		result.Details = append(result.Details,
			"Rook needs raw disks or partitions without a filesystem. For OSDs on PVCs, use --pvc-cluster. See https://rook.io/docs/rook/latest/Getting-Started/Prerequisites/prerequisites/")
	case nodesWithDisks < defaultReplicaHosts:
		result.Status = health.StatusWarning
		result.Details = append(result.Details, fmt.Sprintf(
			"Ceph keeps %d replicas on different hosts by default, so data stays degraded with disks on fewer nodes. "+
				"For a test cluster, set the pool size to %d.", defaultReplicaHosts, max(nodesWithDisks, 1)))
	default:
		result.Status = health.StatusOK
	}
	return result
}

func deviceDetails(dev device) string {
	if dev.Available {
		return "usable"
	}
	return "not usable: " + strings.Join(dev.RejectedReasons, ", ")
}

func rejectedSummary(devices []device) string {
	var reasons []string
	for _, dev := range devices {
		if dev.Available || dev.SysAPI.Size == 0 {
			continue
		}
		reasons = append(reasons, fmt.Sprintf("%s (%s)", dev.Path, strings.Join(dev.RejectedReasons, ", ")))
	}
	if len(reasons) == 0 {
		return ""
	}
	return "; " + strings.Join(reasons, "; ")
}

func checkMemory(nodes []corev1.Node, probes []nodeProbe) health.CheckResult {
	result := health.CheckResult{Name: CheckMemory, Category: health.CategoryK8sResources}
	usable := usableDevices(probes)

	short := 0
	for _, node := range nodes {
		osds := len(usable[node.Name])
		need := monMemory.DeepCopy()
		for range osds {
			need.Add(osdMemory)
		}
		allocatable := node.Status.Allocatable[corev1.ResourceMemory]
		summary := fmt.Sprintf("%s allocatable, about %s needed for a mon", gibibytes(allocatable), gibibytes(need))
		if osds > 0 {
			summary += fmt.Sprintf(" and OSDs on all %d usable disk(s)", osds)
		}
		result.Items = append(result.Items, health.CheckItem{Name: node.Name, Details: summary})
		if allocatable.Cmp(need) < 0 {
			short++
			result.Details = append(result.Details, fmt.Sprintf("%s: %s", node.Name, summary))
		}
	}
	if short > 0 {
		result.Status = health.StatusWarning
		result.Message = fmt.Sprintf("%d node(s) may not have enough memory for a mon and an OSD on every usable disk", short)
		result.Details = append(result.Details,
			"The estimate assumes an OSD on every usable disk with the default osd_memory_target of 4GiB. Rook creates OSDs only on the disks selected in the CephCluster storage settings, and OSDs can run with less memory by lowering osd_memory_target, at some cost in performance.")
		return result
	}
	result.Status = health.StatusOK
	result.Message = "Nodes have enough memory for a mon and an OSD on every usable disk"
	return result
}

func gibibytes(q resource.Quantity) string {
	return fmt.Sprintf("%.1fGi", float64(q.Value())/(1<<30))
}
