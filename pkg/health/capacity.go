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
	"encoding/json"
	"fmt"

	"github.com/rook/kubectl-rook-ceph/pkg/exec"
	"github.com/rook/kubectl-rook-ceph/pkg/k8sutil"
)

type cephDf struct {
	Stats cephDfStats  `json:"stats"`
	Pools []cephDfPool `json:"pools"`
}

type cephDfStats struct {
	TotalBytes        int64   `json:"total_bytes"`
	TotalUsedRawBytes int64   `json:"total_used_raw_bytes"`
	TotalAvailBytes   int64   `json:"total_avail_bytes"`
	TotalUsedRawRatio float64 `json:"total_used_raw_ratio"`
}

type cephDfPool struct {
	Name  string          `json:"name"`
	ID    int             `json:"id"`
	Stats cephDfPoolStats `json:"stats"`
}

type cephDfPoolStats struct {
	PercentUsed float64 `json:"percent_used"`
	MaxAvail    int64   `json:"max_avail"`
	Stored      int64   `json:"stored"`
}

// capacityHealthChecks lists ceph health check codes related to cluster capacity.
// The bool value indicates membership in the set; true means the code is capacity-related
// and the capacity check status should reflect its severity.
var capacityHealthChecks = map[string]bool{
	"NEARFULL_OSD":      true,
	"BACKFILLFULL":      true,
	"FULL_OSD":          true,
	"POOL_NEARFULL":     true,
	"POOL_BACKFILLFULL": true,
	"POOL_FULL":         true,
}

func checkClusterCapacity(ctx context.Context, clientsets *k8sutil.Clientsets, operatorNamespace, clusterNamespace string, status cephStatus, statusErr error) CheckResult {
	result := CheckResult{
		Name:     CheckClusterCapacity,
		Category: CategoryStorage,
	}

	dfOut, err := exec.RunCommandInOperatorPod(ctx, clientsets, "ceph", []string{"df", "--format", "json"}, operatorNamespace, clusterNamespace, true)
	if err != nil {
		result.Status = StatusError
		result.Message = fmt.Sprintf("Failed to get ceph df: %v", err)
		return result
	}

	var df cephDf
	if err := json.Unmarshal([]byte(dfOut), &df); err != nil {
		result.Status = StatusError
		result.Message = fmt.Sprintf("Failed to parse ceph df output: %v", err)
		return result
	}

	pct := df.Stats.TotalUsedRawRatio * 100
	result.Status = StatusOK
	result.Message = fmt.Sprintf("Cluster capacity %.1f%% used (%s / %s)",
		pct, humanizeBytes(df.Stats.TotalUsedRawBytes), humanizeBytes(df.Stats.TotalBytes))

	if statusErr == nil {
		result.Status = capacityStatusFromHealth(status.Health.Checks)
	}

	for _, pool := range df.Pools {
		poolPct := pool.Stats.PercentUsed * 100
		result.Items = append(result.Items, CheckItem{
			Name:    pool.Name,
			Details: fmt.Sprintf("%.1f%% used, %s stored", poolPct, humanizeBytes(pool.Stats.Stored)),
		})
	}

	return result
}

func capacityStatusFromHealth(checks map[string]healthCheckEntry) CheckStatus {
	status := StatusOK
	for code, check := range checks {
		if !capacityHealthChecks[code] {
			continue
		}
		if check.Severity == "HEALTH_ERR" {
			return StatusCritical
		}
		status = StatusWarning
	}
	return status
}

func humanizeBytes(b int64) string {
	const (
		kib = 1024
		mib = 1024 * kib
		gib = 1024 * mib
		tib = 1024 * gib
	)

	switch {
	case b >= tib:
		return fmt.Sprintf("%.1f TiB", float64(b)/float64(tib))
	case b >= gib:
		return fmt.Sprintf("%.1f GiB", float64(b)/float64(gib))
	case b >= mib:
		return fmt.Sprintf("%.1f MiB", float64(b)/float64(mib))
	case b >= kib:
		return fmt.Sprintf("%.1f KiB", float64(b)/float64(kib))
	default:
		return fmt.Sprintf("%d B", b)
	}
}
