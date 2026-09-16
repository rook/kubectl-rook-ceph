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

func Test_resolveLogTarget(t *testing.T) {
	const (
		operatorNs = "rook-ceph-operator-ns"
		clusterNs  = "my-cluster"
	)

	tests := []struct {
		name         string
		target       string
		selector     string
		wantNs       string
		wantSelector string
		wantErr      string
	}{
		{
			name:         "the operator is read from the operator namespace",
			target:       "operator",
			wantNs:       operatorNs,
			wantSelector: "app=rook-ceph-operator",
		},
		{
			name:         "the exporter is read from the cluster namespace",
			target:       "exporter",
			wantNs:       clusterNs,
			wantSelector: "app=rook-ceph-exporter",
		},
		{
			name:         "crash collector",
			target:       "crashcollector",
			wantNs:       clusterNs,
			wantSelector: "app=rook-ceph-crashcollector",
		},
		{
			name:         "toolbox",
			target:       "toolbox",
			wantNs:       clusterNs,
			wantSelector: "app=rook-ceph-tools",
		},
		{
			name:         "a daemon type matches every daemon of that type",
			target:       "osd",
			wantNs:       clusterNs,
			wantSelector: "ceph_daemon_type=osd",
		},
		{
			name:         "a daemon type and id match a single daemon",
			target:       "mon.a",
			wantNs:       clusterNs,
			wantSelector: "ceph_daemon_type=mon,ceph_daemon_id=a",
		},
		{
			name:         "a numeric daemon id",
			target:       "osd.0",
			wantNs:       clusterNs,
			wantSelector: "ceph_daemon_type=osd,ceph_daemon_id=0",
		},
		{
			name:         "a daemon id containing a dash",
			target:       "mds.myfs-a",
			wantNs:       clusterNs,
			wantSelector: "ceph_daemon_type=mds,ceph_daemon_id=myfs-a",
		},
		{
			name:         "a daemon id containing a dot",
			target:       "rgw.my.store-a",
			wantNs:       clusterNs,
			wantSelector: "ceph_daemon_type=rgw,ceph_daemon_id=my.store-a",
		},
		{
			name:         "a hyphenated daemon type",
			target:       "rbd-mirror",
			wantNs:       clusterNs,
			wantSelector: "ceph_daemon_type=rbd-mirror",
		},
		{
			name:         "the nvmeof gateway",
			target:       "nvmeof",
			wantNs:       clusterNs,
			wantSelector: "ceph_daemon_type=nvmeof",
		},
		{
			name:         "a selector is passed through against the cluster namespace",
			selector:     "app=csi-rbdplugin-provisioner",
			wantNs:       clusterNs,
			wantSelector: "app=csi-rbdplugin-provisioner",
		},
		{
			name:     "a target and a selector are mutually exclusive",
			target:   "mon.a",
			selector: "app=rook-ceph-mon",
			wantErr:  "not both",
		},
		{
			name:    "neither a target nor a selector",
			wantErr: "specify a target or --selector",
		},
		{
			name:    "an unknown target reports the valid ones",
			target:  "mgr-dashboard",
			wantErr: `unknown target "mgr-dashboard"`,
		},
		{
			name:    "a daemon type with an empty id",
			target:  "mon.",
			wantErr: "missing a daemon id",
		},
		{
			name:    "a target that is only a separator",
			target:  ".",
			wantErr: `unknown target "."`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			namespace, selector, err := resolveLogTarget(tt.target, tt.selector, operatorNs, clusterNs)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantNs, namespace)
			assert.Equal(t, tt.wantSelector, selector)
		})
	}
}

func Test_isCephDaemonType(t *testing.T) {
	for _, daemonType := range cephDaemonTypes {
		assert.True(t, isCephDaemonType(daemonType), "expected %q to be a ceph daemon type", daemonType)
	}

	for _, notADaemon := range []string{"", "operator", "mo", "mons", "zzz", "aaa"} {
		assert.False(t, isCephDaemonType(notADaemon), "expected %q not to be a ceph daemon type", notADaemon)
	}
}

func TestCephDaemonTypesAreSorted(t *testing.T) {
	// the list is rendered into the unknown-target error, which should read predictably
	assert.IsIncreasing(t, cephDaemonTypes)
}

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
		{name: "selector", shorthand: "l", defValue: ""},
		{name: "container", shorthand: "c", defValue: ""},
		{name: "all-containers", shorthand: "", defValue: "false"},
		{name: "max-log-requests", shorthand: "", defValue: "50"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flag := LogsCmd.Flags().Lookup(tt.name)
			require.NotNil(t, flag, "expected --%s flag to be registered", tt.name)
			assert.Equal(t, tt.shorthand, flag.Shorthand)
			assert.Equal(t, tt.defValue, flag.DefValue)
		})
	}
}

func TestLogsCmdAcceptsAtMostOneTarget(t *testing.T) {
	assert.NoError(t, LogsCmd.Args(LogsCmd, []string{}))
	assert.NoError(t, LogsCmd.Args(LogsCmd, []string{"mon.a"}))
	assert.Error(t, LogsCmd.Args(LogsCmd, []string{"mon.a", "osd.0"}))
}

func Test_logFlags_validate(t *testing.T) {
	tests := []struct {
		name    string
		flags   logFlags
		wantErr string
	}{
		{name: "defaults", flags: logFlags{tail: -1, maxRequests: 50}},
		{name: "tail of zero lines", flags: logFlags{tail: 0, maxRequests: 50}},
		{name: "positive tail and since", flags: logFlags{tail: 20, since: 5 * time.Minute, maxRequests: 50}},
		{name: "tail below -1", flags: logFlags{tail: -2, maxRequests: 50}, wantErr: "invalid --tail -2"},
		{name: "negative since", flags: logFlags{tail: -1, since: -time.Second, maxRequests: 50}, wantErr: "invalid --since -1s"},
		{name: "max requests below one", flags: logFlags{tail: -1, maxRequests: 0}, wantErr: "invalid --max-log-requests 0"},
		{
			name:    "all containers and a single container",
			flags:   logFlags{tail: -1, maxRequests: 50, allContainers: true, container: "mon"},
			wantErr: "mutually exclusive",
		},
		{name: "all containers alone", flags: logFlags{tail: -1, maxRequests: 50, allContainers: true}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.flags.validate()
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func Test_logFlags_podLogOptions(t *testing.T) {
	tests := []struct {
		name          string
		flags         logFlags
		wantTail      *int64
		wantSinceSecs *int64
	}{
		{
			name:  "defaults leave both limits unset",
			flags: logFlags{tail: -1},
		},
		{
			name:     "explicit tail is passed through",
			flags:    logFlags{tail: 20},
			wantTail: new(int64(20)),
		},
		{
			name:     "tail of zero means zero lines, not all lines",
			flags:    logFlags{tail: 0},
			wantTail: new(int64(0)),
		},
		{
			name:          "since is converted to seconds",
			flags:         logFlags{tail: -1, since: 5 * time.Minute},
			wantSinceSecs: new(int64(300)),
		},
		{
			name:          "sub-second since rounds up so it still limits output",
			flags:         logFlags{tail: -1, since: 400 * time.Millisecond},
			wantSinceSecs: new(int64(1)),
		},
		{
			name:          "fractional since rounds up",
			flags:         logFlags{tail: -1, since: 1500 * time.Millisecond},
			wantSinceSecs: new(int64(2)),
		},
		{
			name:  "boolean flags are passed through",
			flags: logFlags{tail: -1, follow: true, previous: true, timestamps: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := tt.flags.podLogOptions()

			// the container is chosen per stream, not by the flags
			assert.Empty(t, opts.Container)
			assert.Equal(t, tt.flags.follow, opts.Follow)
			assert.Equal(t, tt.flags.previous, opts.Previous)
			assert.Equal(t, tt.flags.timestamps, opts.Timestamps)
			assert.Equal(t, tt.wantTail, opts.TailLines)
			assert.Equal(t, tt.wantSinceSecs, opts.SinceSeconds)
		})
	}
}
