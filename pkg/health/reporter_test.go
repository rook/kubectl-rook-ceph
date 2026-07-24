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
	"io"
	"os"
	"strings"
	"testing"

	"github.com/fatih/color"
	"github.com/stretchr/testify/assert"
)

// captureOutput redirects os.Stderr to a pipe, runs fn, and returns what was printed.
func captureOutput(t *testing.T, fn func()) string {
	t.Helper()

	origNoColor := color.NoColor
	color.NoColor = true
	defer func() { color.NoColor = origNoColor }()

	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	os.Stderr = w

	var out []byte
	var readErr error
	done := make(chan struct{})
	go func() {
		out, readErr = io.ReadAll(r)
		close(done)
	}()

	cleaned := false
	cleanup := func() {
		if cleaned {
			return
		}
		cleaned = true
		_ = w.Close()
		<-done
		_ = r.Close()
		os.Stderr = old
	}
	defer cleanup()

	fn()
	cleanup()

	if readErr != nil {
		t.Fatalf("failed to read captured stderr: %v", readErr)
	}
	return string(out)
}

func TestStatusIcon(t *testing.T) {
	tests := []struct {
		status   CheckStatus
		expected string
	}{
		{StatusOK, "[OK]"},
		{StatusWarning, "[!!]"},
		{StatusCritical, "[XX]"},
		{StatusError, "[XX]"},
		{CheckStatus(99), "[XX]"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expected, statusIcon(tt.status))
	}
}

func TestStatusLabel(t *testing.T) {
	tests := []struct {
		status   CheckStatus
		expected string
	}{
		{StatusOK, "OK"},
		{StatusWarning, "WARNING"},
		{StatusCritical, "CRITICAL"},
		{StatusError, "ERROR"},
		{CheckStatus(99), "ERROR"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expected, statusLabel(tt.status))
	}
}

func TestCountByStatus(t *testing.T) {
	results := []CheckResult{
		{Status: StatusOK},
		{Status: StatusOK},
		{Status: StatusWarning},
		{Status: StatusCritical},
		{Status: StatusError},
	}
	ok, warning, critical, errCount := countByStatus(results)
	assert.Equal(t, 2, ok)
	assert.Equal(t, 1, warning)
	assert.Equal(t, 1, critical)
	assert.Equal(t, 1, errCount)
}

func TestGroupByCategory(t *testing.T) {
	results := []CheckResult{
		{Name: "A", Category: "Storage"},
		{Name: "B", Category: "K8s Resources"},
		{Name: "C", Category: "Storage"},
		{Name: "D", Category: "Network"},
	}
	grouped := groupByCategory(results)
	assert.Len(t, grouped["Storage"], 2)
	assert.Len(t, grouped["K8s Resources"], 1)
	assert.Len(t, grouped["Network"], 1)
}

func TestSeverityOrdering(t *testing.T) {
	assert.Less(t, severityRank(StatusError), severityRank(StatusCritical))
	assert.Less(t, severityRank(StatusCritical), severityRank(StatusWarning))
	assert.Less(t, severityRank(StatusWarning), severityRank(StatusOK))
}

func TestPrintCategorySortsBySeverity(t *testing.T) {
	results := []CheckResult{
		{Name: "OKCheck", Status: StatusOK, Message: "good"},
		{Name: "CritCheck", Status: StatusCritical, Message: "bad"},
		{Name: "WarnCheck", Status: StatusWarning, Message: "meh"},
	}

	output := captureOutput(t, func() {
		printCategory("Storage", results, true)
	})

	critIdx := strings.Index(output, "CritCheck")
	warnIdx := strings.Index(output, "WarnCheck")
	okIdx := strings.Index(output, "OKCheck")

	assert.NotEqual(t, -1, critIdx, "CritCheck not found in output")
	assert.NotEqual(t, -1, warnIdx, "WarnCheck not found in output")
	assert.NotEqual(t, -1, okIdx, "OKCheck not found in output")

	assert.Greater(t, warnIdx, critIdx, "Critical should appear before Warning")
	assert.Greater(t, okIdx, warnIdx, "Warning should appear before OK")
}

func TestPrintCheckResultWithItems(t *testing.T) {
	result := CheckResult{
		Name:    "Mon Distribution",
		Status:  StatusOK,
		Message: "3 mon pods on 3 nodes",
		Items: []CheckItem{
			{Name: "mon-a", Status: "Running", Node: "node1"},
			{Name: "mon-b", Status: "Running", Node: "node2"},
		},
	}

	output := captureOutput(t, func() {
		printCheckResult(result, true)
	})

	assert.Contains(t, output, "[OK]")
	assert.Contains(t, output, "Mon Distribution")
	assert.Contains(t, output, "3 mon pods on 3 nodes")
	assert.Contains(t, output, "mon-a -> node1 (Running)")
	assert.Contains(t, output, "mon-b -> node2 (Running)")
}

func TestPrintCheckResultItemWithoutNode(t *testing.T) {
	result := CheckResult{
		Name:    "SomeCheck",
		Status:  StatusWarning,
		Message: "issue found",
		Items: []CheckItem{
			{Name: "resource-1", Status: "Pending"},
		},
	}

	output := captureOutput(t, func() {
		printCheckResult(result, true)
	})

	assert.Contains(t, output, "resource-1 (Pending)")
	assert.NotContains(t, output, "->")
}

func TestPrintCheckResultWithDetails(t *testing.T) {
	result := CheckResult{
		Name:    "PG Status",
		Status:  StatusOK,
		Message: "128 PGs active+clean",
		Details: []string{"PgState: active+clean, PgCount: 128"},
	}

	output := captureOutput(t, func() {
		printCheckResult(result, true)
	})

	assert.Contains(t, output, "Details:")
	assert.Contains(t, output, "- PgState: active+clean, PgCount: 128")
}

func TestPrintSummaryWithError(t *testing.T) {
	results := []CheckResult{
		{Status: StatusOK},
		{Name: "TestCheck", Status: StatusError, Message: "failed to get ceph status"},
	}

	output := captureOutput(t, func() {
		printSummary(results)
	})

	assert.Contains(t, output, "Total Checks: 2")
	assert.Contains(t, output, "OK:")
	assert.Contains(t, output, "Error:")
	assert.Contains(t, output, "failed")
}

func TestPrintSummaryWithoutError(t *testing.T) {
	results := []CheckResult{
		{Status: StatusOK},
		{Status: StatusWarning},
	}

	output := captureOutput(t, func() {
		printSummary(results)
	})

	assert.Contains(t, output, "Total Checks: 2")
	assert.NotContains(t, output, "Error")
}

func TestPrintReportErrorsOnUnknownCategories(t *testing.T) {
	results := []CheckResult{
		{Name: "Check1", Category: "Storage", Status: StatusOK, Message: "ok"},
		{Name: "Check2", Category: "CustomCategory", Status: StatusWarning, Message: "warn"},
	}

	output := captureOutput(t, func() {
		printReport("test-ns", results, true)
	})

	assert.Contains(t, output, "Storage")
	assert.Contains(t, output, `unknown category "CustomCategory"`)
}

func TestPrintReportFullOutput(t *testing.T) {
	results := []CheckResult{
		{Name: "Ceph Cluster Health", Category: "Storage", Status: StatusOK, Message: "HEALTH_OK"},
		{Name: "Mon Distribution", Category: "K8s Resources", Status: StatusWarning, Message: "2 mon pods on 2 nodes (at least 3 recommended)",
			Items: []CheckItem{
				{Name: "mon-a", Status: "Running", Node: "node1"},
				{Name: "mon-b", Status: "Running", Node: "node1"},
			}},
		{Name: "MGR Status", Category: "K8s Resources", Status: StatusOK, Message: "1 mgr pod(s) running"},
	}

	output := captureOutput(t, func() {
		printReport("rook-ceph", results, true)
	})

	assert.Contains(t, output, "CLUSTER HEALTH REPORT")
	assert.Contains(t, output, "Namespace: rook-ceph")
	assert.Contains(t, output, "Total Checks: 3")
	assert.Contains(t, output, "OK:")
	assert.Contains(t, output, "Warning:")
	assert.NotContains(t, output, "Critical:")

	assert.Contains(t, output, "HEALTH_OK")
	assert.Contains(t, output, "2 mon pods on 2 nodes")
	assert.Contains(t, output, "mon-a -> node1 (Running)")

	storageIdx := strings.Index(output, "Storage")
	computeIdx := strings.Index(output, "K8s Resources")

	assert.NotEqual(t, -1, storageIdx, "Storage category not found in output")
	assert.NotEqual(t, -1, computeIdx, "Compute category not found in output")

	assert.Greater(t, computeIdx, storageIdx, "Storage should appear before Compute")

	monIdx := strings.Index(output, "Mon Distribution")
	mgrIdx := strings.Index(output, "MGR Status")

	assert.NotEqual(t, -1, monIdx, "MonDistribution not found in output")
	assert.NotEqual(t, -1, mgrIdx, "MGRStatus not found in output")

	assert.Greater(t, mgrIdx, monIdx, "Warning checks should appear before OK within Compute")

	summaryIdx := strings.Index(output, "SUMMARY")
	assert.NotEqual(t, -1, summaryIdx, "SUMMARY not found in output")
	assert.Greater(t, summaryIdx, computeIdx, "Summary should appear after all categories")
}

func TestPrintCheckResultVerboseFalseHidesItems(t *testing.T) {
	result := CheckResult{
		Name:    "Mon Distribution",
		Status:  StatusOK,
		Message: "3 mon pods on 3 nodes",
		Details: []string{"some detail"},
		Items: []CheckItem{
			{Name: "mon-a", Status: "Running", Node: "node1"},
		},
	}

	output := captureOutput(t, func() {
		printCheckResult(result, false)
	})

	assert.Contains(t, output, "Mon Distribution")
	assert.Contains(t, output, "3 mon pods on 3 nodes")
	assert.Contains(t, output, "some detail")
	assert.NotContains(t, output, "Items:")
	assert.NotContains(t, output, "mon-a")
}

func TestPrintCheckResultVerboseTrueShowsItems(t *testing.T) {
	result := CheckResult{
		Name:    "Mon Distribution",
		Status:  StatusOK,
		Message: "3 mon pods on 3 nodes",
		Items: []CheckItem{
			{Name: "mon-a", Status: "Running", Node: "node1"},
		},
	}

	output := captureOutput(t, func() {
		printCheckResult(result, true)
	})

	assert.Contains(t, output, "mon-a -> node1 (Running)")
}
