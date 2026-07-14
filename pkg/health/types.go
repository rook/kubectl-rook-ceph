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

// CheckStatus represents the severity level of a health check result.
type CheckStatus int

const (
	StatusOK CheckStatus = iota
	StatusWarning
	StatusCritical
	StatusError
)

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
	Name     string
	Category string
	Status   CheckStatus
	Message  string
	Details  []string
	Items    []CheckItem
}

// CheckItem represents an individual resource within a check.
type CheckItem struct {
	Name      string
	Namespace string
	Status    string
	Node      string
	Details   string
}
