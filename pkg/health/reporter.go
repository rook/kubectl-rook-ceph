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
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/fatih/color"
	"github.com/rook/kubectl-rook-ceph/pkg/logging"
	"gopkg.in/yaml.v3"
)

var categoryOrder = []string{CategoryStorage, CategoryK8sResources, CategoryNetwork, CategoryObjectStorage}

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
	logging.Plain("")
	logging.Plain("%s", separator())
	logging.Plain("CLUSTER HEALTH REPORT")
	logging.Plain("%s", separator())
	printAligned(
		fmt.Sprintf("Generated:\t%s", time.Now().UTC().Format("2006-01-02 15:04:05 UTC")),
		fmt.Sprintf("Namespace:\t%s", clusterNamespace),
	)
}

func printSummary(results []CheckResult) {
	ok, warning, critical, errCount := countByStatus(results)
	logging.Plain("")
	logging.Plain("%s", separator())
	logging.Plain("SUMMARY")
	logging.Plain("%s", separator())
	var lines []string
	lines = append(lines, fmt.Sprintf("Total Checks:\t%d", len(results)))
	if ok > 0 {
		lines = append(lines, fmt.Sprintf("OK:\t%d", ok))
	}
	if warning > 0 {
		lines = append(lines, fmt.Sprintf("Warning:\t%d", warning))
	}
	if critical > 0 {
		lines = append(lines, fmt.Sprintf("Critical:\t%d", critical))
	}
	if errCount > 0 {
		lines = append(lines, fmt.Sprintf("Error:\t%d", errCount))
	}
	printAligned(lines...)
	for _, r := range results {
		if r.Status == StatusError {
			logging.Error(fmt.Errorf("check %s failed: %s", r.Name, r.Message))
		}
	}
}

func printCategory(name string, results []CheckResult, verbose bool) {
	sort.SliceStable(results, func(i, j int) bool {
		return severityRank(results[i].Status) < severityRank(results[j].Status)
	})

	logging.Plain("")
	logging.Plain("%s", separator())
	logging.Plain("%s", name)
	logging.Plain("%s", separator())

	for _, r := range results {
		printCheckResult(r, verbose)
	}
}

func printCheckResult(r CheckResult, verbose bool) {
	colorFn := statusColor(r.Status)
	icon := colorFn(statusIcon(r.Status))
	label := colorFn(statusLabel(r.Status))
	logging.Plain("")
	logging.Plain("%s %s [%s]", icon, r.Name, label)
	logging.Plain("\tStatus: %s", r.Message)

	if len(r.Details) > 0 {
		logging.Plain("\tDetails:")
		for _, d := range r.Details {
			logging.Plain("\t\t- %s", d)
		}
	}

	if verbose && len(r.Items) > 0 {
		for _, item := range r.Items {
			label := item.Name
			if item.Namespace != "" {
				label = path.Join(item.Namespace, item.Name)
			}
			if item.Node != "" {
				logging.Plain("\t\t- %s -> %s (%s)", label, item.Node, item.Status)
			} else {
				logging.Plain("\t\t- %s (%s)", label, item.Status)
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

func printAligned(lines ...string) {
	var buf bytes.Buffer
	tw := tabwriter.NewWriter(&buf, 0, 0, 1, ' ', 0)
	for _, line := range lines {
		fmt.Fprintln(tw, line)
	}
	tw.Flush()
	for _, line := range strings.Split(strings.TrimRight(buf.String(), "\n"), "\n") {
		logging.Plain("%s", line)
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
	case StatusCritical, StatusError:
		return "[XX]"
	default:
		return "[XX]"
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
	case StatusError:
		return "ERROR"
	default:
		return "ERROR"
	}
}

func severityRank(s CheckStatus) int {
	switch s {
	case StatusError:
		return 0
	case StatusCritical:
		return 1
	case StatusWarning:
		return 2
	case StatusOK:
		return 3
	default:
		return 4
	}
}

func countByStatus(results []CheckResult) (ok, warning, critical, errCount int) {
	for _, r := range results {
		switch r.Status {
		case StatusOK:
			ok++
		case StatusWarning:
			warning++
		case StatusCritical:
			critical++
		default:
			errCount++
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

func formatReport(clusterNamespace string, results []CheckResult, format string, verbose bool) {
	switch format {
	case "json":
		report := buildReport(clusterNamespace, results)
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			logging.Fatal(fmt.Errorf("failed to marshal JSON report: %v", err))
		}
		fmt.Fprintln(os.Stdout, string(data))
	case "yaml":
		report := buildReport(clusterNamespace, results)
		data, err := yaml.Marshal(report)
		if err != nil {
			logging.Fatal(fmt.Errorf("failed to marshal YAML report: %v", err))
		}
		fmt.Fprint(os.Stdout, string(data))
	default:
		printReport(clusterNamespace, results, verbose)
	}
}

func buildReport(clusterNamespace string, results []CheckResult) HealthReport {
	ok, warning, critical, errCount := countByStatus(results)
	return HealthReport{
		Namespace: clusterNamespace,
		Timestamp: time.Now().UTC(),
		Checks:    results,
		Summary: ReportSummary{
			Total:    len(results),
			OK:       ok,
			Warning:  warning,
			Critical: critical,
			Error:    errCount,
		},
	}
}
