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
	"encoding/json"
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

// CheckStatus represents the severity level of a health check result.
type CheckStatus int

const (
	StatusOK CheckStatus = iota
	StatusWarning
	StatusCritical
	StatusError
)

var statusStrings = map[CheckStatus]string{
	StatusOK:       "ok",
	StatusWarning:  "warning",
	StatusCritical: "critical",
	StatusError:    "error",
}

func (s CheckStatus) String() string {
	if str, ok := statusStrings[s]; ok {
		return str
	}
	return "error"
}

func (s CheckStatus) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

func (s *CheckStatus) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}
	for k, v := range statusStrings {
		if v == str {
			*s = k
			return nil
		}
	}
	return fmt.Errorf("unknown status: %q", str)
}

func (s CheckStatus) MarshalYAML() (interface{}, error) {
	return s.String(), nil
}

func (s *CheckStatus) UnmarshalYAML(value *yaml.Node) error {
	for k, v := range statusStrings {
		if v == value.Value {
			*s = k
			return nil
		}
	}
	return fmt.Errorf("unknown status: %q", value.Value)
}

const (
	CategoryStorage       = "Storage"
	CategoryK8sResources  = "K8s Resources"
	CategoryNetwork       = "Network"
	CategoryObjectStorage = "Object Storage"

	CheckMonDistribution   = "Mon Distribution"
	CheckCephClusterHealth = "Ceph Cluster Health"
	CheckOSDDistribution   = "OSD Distribution"
	CheckAllPodsStatus     = "Pods Status"
	CheckPGStatus          = "PG Status"
	CheckMGRStatus         = "MGR Status"
)

// CheckResult represents the outcome of a single health check.
type CheckResult struct {
	Name     string      `json:"name" yaml:"name"`
	Category string      `json:"category" yaml:"category"`
	Status   CheckStatus `json:"status" yaml:"status"`
	Message  string      `json:"message" yaml:"message"`
	Details  []string    `json:"details,omitempty" yaml:"details,omitempty"`
	Items    []CheckItem `json:"items,omitempty" yaml:"items,omitempty"`
}

// CheckItem represents an individual resource within a check.
type CheckItem struct {
	Name      string `json:"name" yaml:"name"`
	Namespace string `json:"namespace,omitempty" yaml:"namespace,omitempty"`
	Status    string `json:"status,omitempty" yaml:"status,omitempty"`
	Node      string `json:"node,omitempty" yaml:"node,omitempty"`
	Details   string `json:"details,omitempty" yaml:"details,omitempty"`
}

// HealthReport is the top-level structure for JSON/YAML output.
type HealthReport struct {
	Namespace string        `json:"namespace" yaml:"namespace"`
	Timestamp time.Time     `json:"timestamp" yaml:"timestamp"`
	Checks    []CheckResult `json:"checks" yaml:"checks"`
	Summary   ReportSummary `json:"summary" yaml:"summary"`
}

// ReportSummary counts check results by status.
type ReportSummary struct {
	Total    int `json:"total" yaml:"total"`
	OK       int `json:"ok" yaml:"ok"`
	Warning  int `json:"warning" yaml:"warning"`
	Critical int `json:"critical" yaml:"critical"`
	Error    int `json:"error" yaml:"error"`
}
