/*
Copyright 2023 The Rook Authors. All rights reserved.

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
	"fmt"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/rook/kubectl-rook-ceph/pkg/logging"
)

var categoryOrder = []string{CategoryStorage, CategoryCompute, CategoryNetwork, CategoryObjectStorage}

func printReport(clusterNamespace string, results []CheckResult, verbose bool) {
	printHeader(clusterNamespace)

	grouped := groupByCategory(results)
	for _, category := range categoryOrder {
		if checks, ok := grouped[category]; ok {
			printCategory(category, checks, verbose)
			delete(grouped, category)
		}
	}
	for category := range grouped {
		logging.Error(fmt.Errorf("unknown category %q found in health check results", category))
	}

	printSummary(results)
}

func printHeader(clusterNamespace string) {
	logging.Info("")
	logging.Info("%s", separator())
	logging.Info("CLUSTER HEALTH REPORT")
	logging.Info("%s", separator())
	logging.Info("Generated: %s", time.Now().UTC().Format("2006-01-02 15:04:05 UTC"))
	logging.Info("Namespace: %s", clusterNamespace)
}

func printSummary(results []CheckResult) {
	ok, warning, critical, _ := countByStatus(results)
	logging.Info("")
	logging.Info("%s", separator())
	logging.Info("SUMMARY")
	logging.Info("%s", separator())
	logging.Info("Total Checks: %d", len(results))
	if ok > 0 {
		logging.Info("OK:           %d", ok)
	}
	if warning > 0 {
		logging.Info("Warning:      %d", warning)
	}
	if critical > 0 {
		logging.Info("Critical:     %d", critical)
	}
	for _, r := range results {
		if r.Status == StatusUnknown {
			logging.Error(fmt.Errorf("check %s could not determine status: %s", r.Name, r.Message))
		}
	}
}

func printCategory(name string, results []CheckResult, verbose bool) {
	sort.SliceStable(results, func(i, j int) bool {
		return severityRank(results[i].Status) < severityRank(results[j].Status)
	})

	logging.Info("")
	logging.Info("%s", separator())
	logging.Info("%s", name)
	logging.Info("%s", separator())

	for _, r := range results {
		printCheckResult(r, verbose)
	}
}

func printCheckResult(r CheckResult, verbose bool) {
	colorFn := statusColor(r.Status)
	icon := colorFn(statusIcon(r.Status))
	label := colorFn(statusLabel(r.Status))
	logging.Info("")
	logging.Info("%s %-55s [%s]", icon, r.Name, label)
	logging.Info("   Status: %s", r.Message)

	if len(r.Details) > 0 {
		logging.Info("   Details:")
		for _, d := range r.Details {
			logging.Info("     - %s", d)
		}
	}

	if verbose && len(r.Items) > 0 {
		for _, item := range r.Items {
			label := item.Name
			if item.Namespace != "" {
				label = path.Join(item.Namespace, item.Name)
			}
			if item.Node != "" {
				logging.Info("     - %s -> %s (%s)", label, item.Node, item.Status)
			} else {
				logging.Info("     - %s (%s)", label, item.Status)
			}
		}
	}
}

func statusColor(s CheckStatus) func(a ...interface{}) string {
	switch s {
	case StatusOK:
		return color.New(color.FgGreen).SprintFunc()
	case StatusWarning:
		return color.New(color.FgYellow).SprintFunc()
	case StatusCritical:
		return color.New(color.FgRed).SprintFunc()
	default:
		return color.New(color.FgRed).SprintFunc()
	}
}

func separator() string {
	return strings.Repeat("=", 72)
}

func statusIcon(s CheckStatus) string {
	switch s {
	case StatusOK:
		return "[OK]"
	case StatusWarning:
		return "[!!]"
	case StatusCritical:
		return "[XX]"
	default:
		return "[??]"
	}
}

func statusLabel(s CheckStatus) string {
	switch s {
	case StatusOK:
		return "OK"
	case StatusWarning:
		return "WARNING"
	case StatusCritical:
		return "CRITICAL"
	default:
		return "UNKNOWN"
	}
}

func severityRank(s CheckStatus) int {
	switch s {
	case StatusCritical:
		return 0
	case StatusWarning:
		return 1
	case StatusUnknown:
		return 2
	case StatusOK:
		return 3
	default:
		return 4
	}
}

func countByStatus(results []CheckResult) (ok, warning, critical, unknown int) {
	for _, r := range results {
		switch r.Status {
		case StatusOK:
			ok++
		case StatusWarning:
			warning++
		case StatusCritical:
			critical++
		default:
			unknown++
		}
	}
	return
}

func groupByCategory(results []CheckResult) map[string][]CheckResult {
	grouped := make(map[string][]CheckResult)
	for _, r := range results {
		grouped[r.Category] = append(grouped[r.Category], r)
	}
	return grouped
}
