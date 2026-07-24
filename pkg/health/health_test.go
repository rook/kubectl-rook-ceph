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
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWorseStatus(t *testing.T) {
	tests := []struct {
		name     string
		a, b     CheckStatus
		expected CheckStatus
	}{
		{"OK+OK", StatusOK, StatusOK, StatusOK},
		{"OK+Warning", StatusOK, StatusWarning, StatusWarning},
		{"OK+Critical", StatusOK, StatusCritical, StatusCritical},
		{"OK+Error", StatusOK, StatusError, StatusError},
		{"Warning+OK", StatusWarning, StatusOK, StatusWarning},
		{"Warning+Warning", StatusWarning, StatusWarning, StatusWarning},
		{"Warning+Critical", StatusWarning, StatusCritical, StatusCritical},
		{"Warning+Error", StatusWarning, StatusError, StatusError},
		{"Critical+OK", StatusCritical, StatusOK, StatusCritical},
		{"Critical+Warning", StatusCritical, StatusWarning, StatusCritical},
		{"Critical+Critical", StatusCritical, StatusCritical, StatusCritical},
		{"Critical+Error", StatusCritical, StatusError, StatusError},
		{"Error+OK", StatusError, StatusOK, StatusError},
		{"Error+Warning", StatusError, StatusWarning, StatusError},
		{"Error+Critical", StatusError, StatusCritical, StatusError},
		{"Error+Error", StatusError, StatusError, StatusError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, worseStatus(tt.a, tt.b))
		})
	}
}

func TestEvaluateMonQuorum(t *testing.T) {
	tests := []struct {
		name           string
		quorumNames    []string
		mons           []monEntry
		totalMons      int
		expectedStatus CheckStatus
		expectContains []string
	}{
		{
			name:           "all mons in quorum",
			quorumNames:    []string{"a", "b", "c"},
			mons:           []monEntry{{Name: "a"}, {Name: "b"}, {Name: "c"}},
			totalMons:      3,
			expectedStatus: StatusOK,
			expectContains: []string{"3/3 mons in quorum"},
		},
		{
			name:           "one mon out of quorum, majority holds",
			quorumNames:    []string{"a", "b"},
			mons:           []monEntry{{Name: "a"}, {Name: "b"}, {Name: "c"}},
			totalMons:      3,
			expectedStatus: StatusWarning,
			expectContains: []string{"2/3 mons in quorum", "Mon c not in quorum"},
		},
		{
			name:           "quorum lost",
			quorumNames:    []string{"a"},
			mons:           []monEntry{{Name: "a"}, {Name: "b"}, {Name: "c"}},
			totalMons:      3,
			expectedStatus: StatusCritical,
			expectContains: []string{"1/3 mons in quorum", "Mon b not in quorum", "Mon c not in quorum"},
		},
		{
			name:           "empty monmap",
			quorumNames:    nil,
			mons:           nil,
			totalMons:      0,
			expectedStatus: StatusOK,
			expectContains: []string{"Mon quorum data not available in ceph status"},
		},
		{
			name:           "five mons, two out",
			quorumNames:    []string{"a", "b", "c"},
			mons:           []monEntry{{Name: "a"}, {Name: "b"}, {Name: "c"}, {Name: "d"}, {Name: "e"}},
			totalMons:      5,
			expectedStatus: StatusWarning,
			expectContains: []string{"3/5 mons in quorum", "Mon d not in quorum", "Mon e not in quorum"},
		},
		{
			name:           "five mons, three out, quorum lost",
			quorumNames:    []string{"a", "b"},
			mons:           []monEntry{{Name: "a"}, {Name: "b"}, {Name: "c"}, {Name: "d"}, {Name: "e"}},
			totalMons:      5,
			expectedStatus: StatusCritical,
			expectContains: []string{"2/5 mons in quorum"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, details := evaluateMonQuorum(tt.quorumNames, tt.mons, tt.totalMons)
			assert.Equal(t, tt.expectedStatus, status)
			for _, expected := range tt.expectContains {
				assert.Contains(t, details, expected)
			}
		})
	}
}
