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
	"fmt"
	"maps"
	"math"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"

	k8sutil "github.com/rook/kubectl-rook-ceph/pkg/k8sutil"
	"github.com/rook/kubectl-rook-ceph/pkg/logging"
	"github.com/spf13/cobra"

	corev1 "k8s.io/api/core/v1"
)

type logFlags struct {
	follow        bool
	previous      bool
	timestamps    bool
	tail          int64
	since         time.Duration
	selector      string
	container     string
	allContainers bool
	maxRequests   int
}

var (
	logs         logFlags
	logNamespace string
	logSelector  string
)

// logTargetAliases name the pods that are not addressable as ceph daemons: the operator, and the
// components Rook labels by app name only.
var logTargetAliases = map[string]struct {
	selector          string
	description       string
	operatorNamespace bool
}{
	"operator":       {selector: "app=rook-ceph-operator", description: "the rook-ceph operator", operatorNamespace: true},
	"exporter":       {selector: "app=rook-ceph-exporter", description: "the ceph exporter on every node"},
	"crashcollector": {selector: "app=rook-ceph-crashcollector", description: "the crash collector on every node"},
	"toolbox":        {selector: "app=rook-ceph-tools", description: "the rook toolbox"},
}

// cephDaemonTypes are the values Rook writes to the ceph_daemon_type pod label. Kept sorted so that
// the help text reads predictably.
var cephDaemonTypes = []string{"fs-mirror", "mds", "mgr", "mon", "nfs", "nvmeof", "osd", "rbd-mirror", "rgw"}

var LogsCmd = &cobra.Command{
	Use:   "logs [target]",
	Short: "Print the logs of Rook-Ceph pods",
	Long: `Print or stream the logs of Rook-Ceph pods.

The target is either a named component, a ceph daemon type, or a ceph daemon type and id:

  operator, exporter, crashcollector, toolbox
  mon, mgr, osd, mds, rgw, nfs, nvmeof, rbd-mirror, fs-mirror
  mon.a, osd.0, mds.myfs-a

Pods that are not addressable this way are reachable with --selector, which reads the ceph cluster
namespace. The CSI drivers run alongside the operator, so reading their logs needs the operator
namespace named explicitly with --namespace.`,
	Example: `  kubectl rook-ceph logs operator -f
  kubectl rook-ceph logs mon.a --tail 20
  kubectl rook-ceph logs osd -f
  kubectl rook-ceph logs osd.0 -c activate
  kubectl rook-ceph logs -n rook-ceph -l "app=rook-ceph.cephfs.csi.ceph.com-ctrlplugin"`,
	Args: cobra.MaximumNArgs(1),
	PreRunE: func(_ *cobra.Command, args []string) error {
		if err := logs.validate(); err != nil {
			return err
		}

		var target string
		if len(args) == 1 {
			target = args[0]
		}

		var err error
		logNamespace, logSelector, err = resolveLogTarget(target, logs.selector, operatorNamespace, cephClusterNamespace)

		return err
	},
	Run: func(cmd *cobra.Command, _ []string) {
		// Interrupting a --follow is the normal way to end it, so it unwinds rather than killing the
		// process: streams that are mid-line still flush what they have read.
		ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		err := k8sutil.StreamLogs(ctx, clientSets.Kube, k8sutil.LogStreamOptions{
			Namespace:      logNamespace,
			LabelSelector:  logSelector,
			Container:      logs.container,
			AllContainers:  logs.allContainers,
			PodLogOptions:  logs.podLogOptions(),
			MaxLogRequests: logs.maxRequests,
			Out:            os.Stdout,
		})
		if err != nil {
			logging.Fatal(err)
		}
	},
}

func init() {
	LogsCmd.Flags().BoolVarP(&logs.follow, "follow", "f", false, "specify if the logs should be streamed")
	LogsCmd.Flags().BoolVarP(&logs.previous, "previous", "p", false, "print the logs for the previous instance of the container if it crashed")
	LogsCmd.Flags().BoolVar(&logs.timestamps, "timestamps", false, "include timestamps on each line in the log output")
	LogsCmd.Flags().Int64Var(&logs.tail, "tail", -1, "lines of recent log file to display, defaults to all")
	LogsCmd.Flags().DurationVar(&logs.since, "since", 0, "only return logs newer than a relative duration like 5s, 2m, or 3h")
	LogsCmd.Flags().StringVarP(&logs.selector, "selector", "l", "", "label selector to read logs from, instead of a target")
	LogsCmd.Flags().StringVarP(&logs.container, "container", "c", "", "read only this container, which may be an init or ephemeral container")
	LogsCmd.Flags().BoolVar(&logs.allContainers, "all-containers", false, "also read the init and ephemeral containers of the matching pods")
	LogsCmd.Flags().IntVar(&logs.maxRequests, "max-log-requests", k8sutil.DefaultMaxLogRequests, "maximum number of log streams to open at once")
}

func (f *logFlags) validate() error {
	if f.tail < -1 {
		return fmt.Errorf("invalid --tail %d, must be greater than or equal to -1", f.tail)
	}
	if f.since < 0 {
		return fmt.Errorf("invalid --since %s, must not be negative", f.since)
	}
	if f.maxRequests < 1 {
		return fmt.Errorf("invalid --max-log-requests %d, must be greater than or equal to 1", f.maxRequests)
	}
	if f.allContainers && f.container != "" {
		return fmt.Errorf("--all-containers and --container are mutually exclusive")
	}

	return nil
}

func (f *logFlags) podLogOptions() *corev1.PodLogOptions {
	opts := &corev1.PodLogOptions{
		Follow:     f.follow,
		Previous:   f.previous,
		Timestamps: f.timestamps,
	}
	if f.tail >= 0 {
		opts.TailLines = &f.tail
	}
	if f.since > 0 {
		// round up: truncating a sub-second duration to 0 would mean "no limit" rather than "almost none"
		sinceSeconds := int64(math.Ceil(f.since.Seconds()))
		opts.SinceSeconds = &sinceSeconds
	}

	return opts
}

// resolveLogTarget maps a target, or an explicit label selector, onto the namespace and label
// selector to read logs from. Daemons are addressed by the labels Rook puts on every daemon pod, so
// that a new daemon type needs no mapping here beyond its name.
func resolveLogTarget(target, selector, operatorNamespace, clusterNamespace string) (string, string, error) {
	if target != "" && selector != "" {
		return "", "", fmt.Errorf("specify either the %q target or --selector, not both", target)
	}
	if target == "" {
		if selector == "" {
			return "", "", fmt.Errorf("specify a target or --selector. %s", logTargetHelp())
		}

		return clusterNamespace, selector, nil
	}

	if alias, ok := logTargetAliases[target]; ok {
		if alias.operatorNamespace {
			return operatorNamespace, alias.selector, nil
		}

		return clusterNamespace, alias.selector, nil
	}

	daemonType, daemonID, hasID := strings.Cut(target, ".")
	if !isCephDaemonType(daemonType) {
		return "", "", fmt.Errorf("unknown target %q. %s", target, logTargetHelp())
	}
	if hasID && daemonID == "" {
		return "", "", fmt.Errorf("target %q is missing a daemon id, use %q for a single daemon or %q for all of them", target, daemonType+".a", daemonType)
	}

	labelSelector := fmt.Sprintf("ceph_daemon_type=%s", daemonType)
	if hasID {
		labelSelector = fmt.Sprintf("%s,ceph_daemon_id=%s", labelSelector, daemonID)
	}

	return clusterNamespace, labelSelector, nil
}

func isCephDaemonType(daemonType string) bool {
	return slices.Contains(cephDaemonTypes, daemonType)
}

func logTargetHelp() string {
	return fmt.Sprintf("Expected one of %s, or a ceph daemon type (%s) optionally followed by an id, such as \"mon.a\"",
		strings.Join(slices.Sorted(maps.Keys(logTargetAliases)), ", "), strings.Join(cephDaemonTypes, ", "))
}
