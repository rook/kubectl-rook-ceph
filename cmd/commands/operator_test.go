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

package command

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogsCmdFlags(t *testing.T) {
	tests := []struct {
		name      string
		shorthand string
		defValue  string
	}{
		{name: "follow", shorthand: "f", defValue: "false"},
		{name: "previous", shorthand: "p", defValue: "false"},
		{name: "timestamps", shorthand: "", defValue: "false"},
		{name: "tail", shorthand: "", defValue: "-1"},
		{name: "since", shorthand: "", defValue: "0s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flag := logsCmd.Flags().Lookup(tt.name)
			require.NotNil(t, flag, "expected --%s flag to be registered", tt.name)
			assert.Equal(t, tt.shorthand, flag.Shorthand)
			assert.Equal(t, tt.defValue, flag.DefValue)
		})
	}
}

func TestLogsCmdRegistered(t *testing.T) {
	for _, cmd := range OperatorCmd.Commands() {
		if cmd.Name() == "logs" {
			return
		}
	}
	t.Fatal("expected the logs sub-command to be registered on OperatorCmd")
}

func Test_validateLogsFlags(t *testing.T) {
	tests := []struct {
		name    string
		tail    int64
		since   time.Duration
		wantErr string
	}{
		{name: "defaults", tail: -1, since: 0},
		{name: "tail of zero lines", tail: 0, since: 0},
		{name: "positive tail and since", tail: 20, since: 5 * time.Minute},
		{name: "tail below -1", tail: -2, wantErr: "invalid --tail -2"},
		{name: "negative since", tail: -1, since: -time.Second, wantErr: "invalid --since -1s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateLogsFlags(tt.tail, tt.since)
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func Test_logOptions(t *testing.T) {
	tests := []struct {
		name          string
		follow        bool
		previous      bool
		timestamps    bool
		tail          int64
		since         time.Duration
		wantTail      *int64
		wantSinceSecs *int64
	}{
		{
			name: "defaults leave both limits unset",
			tail: -1,
		},
		{
			name:     "explicit tail is passed through",
			tail:     20,
			wantTail: ptr(int64(20)),
		},
		{
			name:     "tail of zero means zero lines, not all lines",
			tail:     0,
			wantTail: ptr(int64(0)),
		},
		{
			name:          "since is converted to seconds",
			tail:          -1,
			since:         5 * time.Minute,
			wantSinceSecs: ptr(int64(300)),
		},
		{
			name:          "sub-second since rounds up so it still limits output",
			tail:          -1,
			since:         400 * time.Millisecond,
			wantSinceSecs: ptr(int64(1)),
		},
		{
			name:          "fractional since rounds up",
			tail:          -1,
			since:         1500 * time.Millisecond,
			wantSinceSecs: ptr(int64(2)),
		},
		{
			name:       "boolean flags are passed through",
			follow:     true,
			previous:   true,
			timestamps: true,
			tail:       -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := logOptions(tt.follow, tt.previous, tt.timestamps, tt.tail, tt.since)

			assert.Equal(t, "rook-ceph-operator", opts.Container)
			assert.Equal(t, tt.follow, opts.Follow)
			assert.Equal(t, tt.previous, opts.Previous)
			assert.Equal(t, tt.timestamps, opts.Timestamps)
			assert.Equal(t, tt.wantTail, opts.TailLines)
			assert.Equal(t, tt.wantSinceSecs, opts.SinceSeconds)
		})
	}
}

func ptr(v int64) *int64 {
	return &v
}
