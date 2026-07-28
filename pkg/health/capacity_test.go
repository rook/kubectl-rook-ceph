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
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHumanizeBytes(t *testing.T) {
	tests := []struct {
		name     string
		bytes    int64
		expected string
	}{
		{"zero", 0, "0 B"},
		{"bytes", 512, "512 B"},
		{"kibibytes", 1536, "1.5 KiB"},
		{"mebibytes", 10 * 1024 * 1024, "10.0 MiB"},
		{"gibibytes", 3 * 1024 * 1024 * 1024, "3.0 GiB"},
		{"tebibytes", 2 * 1024 * 1024 * 1024 * 1024, "2.0 TiB"},
		{"large tebibytes", 15 * 1024 * 1024 * 1024 * 1024, "15.0 TiB"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, humanizeBytes(tt.bytes))
		})
	}
}

func TestParseCephDf(t *testing.T) {
	rawJSON := `{
		"stats": {
			"total_bytes": 1099511627776,
			"total_used_raw_bytes": 549755813888,
			"total_avail_bytes": 549755813888,
			"total_used_raw_ratio": 0.5
		},
		"pools": [
			{
				"name": "replicapool",
				"id": 1,
				"stats": {
					"percent_used": 0.45,
					"max_avail": 300000000000,
					"stored": 200000000000
				}
			},
			{
				"name": ".mgr",
				"id": 2,
				"stats": {
					"percent_used": 0.01,
					"max_avail": 500000000000,
					"stored": 5000000
				}
			}
		]
	}`

	var df cephDf
	err := json.Unmarshal([]byte(rawJSON), &df)
	require.NoError(t, err)

	assert.Equal(t, int64(1099511627776), df.Stats.TotalBytes)
	assert.InDelta(t, 0.5, df.Stats.TotalUsedRawRatio, 0.001)
	assert.Len(t, df.Pools, 2)
	assert.Equal(t, "replicapool", df.Pools[0].Name)
	assert.InDelta(t, 0.45, df.Pools[0].Stats.PercentUsed, 0.001)
}

func TestCapacityStatusFromHealth(t *testing.T) {
	tests := []struct {
		name     string
		checks   map[string]healthCheckEntry
		expected CheckStatus
	}{
		{
			"no checks",
			nil,
			StatusOK,
		},
		{
			"unrelated warning",
			map[string]healthCheckEntry{
				"TOO_FEW_OSDS": {Severity: "HEALTH_WARN"},
			},
			StatusOK,
		},
		{
			"nearfull warning",
			map[string]healthCheckEntry{
				"NEARFULL_OSD": {Severity: "HEALTH_WARN"},
			},
			StatusWarning,
		},
		{
			"full error",
			map[string]healthCheckEntry{
				"FULL_OSD": {Severity: "HEALTH_ERR"},
			},
			StatusCritical,
		},
		{
			"mixed nearfull and full",
			map[string]healthCheckEntry{
				"NEARFULL_OSD": {Severity: "HEALTH_WARN"},
				"POOL_FULL":    {Severity: "HEALTH_ERR"},
			},
			StatusCritical,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, capacityStatusFromHealth(tt.checks))
		})
	}
}
