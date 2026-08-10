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

package command

import (
	"fmt"
	"math"
	"os"
	"time"

	k8sutil "github.com/rook/kubectl-rook-ceph/pkg/k8sutil"
	"github.com/rook/kubectl-rook-ceph/pkg/logging"
	"github.com/spf13/cobra"

	corev1 "k8s.io/api/core/v1"
)

var (
	logsFollow     bool
	logsPrevious   bool
	logsTimestamps bool
	logsTail       int64
	logsSince      time.Duration
)

// OperatorCmd represents the operator commands
var OperatorCmd = &cobra.Command{
	Use:                "operator",
	Short:              "Calls subcommands like `restart`  and `set <key> <value>` to update  rook-ceph-operator-config configmap",
	DisableFlagParsing: true,
	Args:               cobra.ExactArgs(1),
}

var restartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart rook-ceph-operator pod",
	Args:  cobra.NoArgs,
	PreRun: func(cmd *cobra.Command, args []string) {
		verifyOperatorPodIsRunning(cmd.Context(), clientSets)
	},
	Run: func(cmd *cobra.Command, _ []string) {
		k8sutil.RestartDeployment(cmd.Context(), clientSets.Kube, operatorNamespace, "rook-ceph-operator")
	},
}

var setCmd = &cobra.Command{
	Use:     "set",
	Short:   "Set the property in the rook-ceph-operator-config configmap.",
	Example: "kubectl rook-ceph operator set <KEY> <VALUE>",
	Args:    cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		k8sutil.UpdateConfigMap(cmd.Context(), clientSets.Kube, operatorNamespace, "rook-ceph-operator-config", args[0], args[1])
	},
}

var logsCmd = &cobra.Command{
	Use:     "logs",
	Short:   "Print the logs of the rook-ceph operator pod",
	Args:    cobra.NoArgs,
	Example: "kubectl rook-ceph operator logs -f",
	PreRunE: func(_ *cobra.Command, _ []string) error {
		return validateLogsFlags(logsTail, logsSince)
	},
	Run: func(cmd *cobra.Command, _ []string) {
		opts := logOptions(logsFollow, logsPrevious, logsTimestamps, logsTail, logsSince)
		err := k8sutil.StreamPodLogs(cmd.Context(), clientSets.Kube, operatorNamespace, "app=rook-ceph-operator", opts, os.Stdout)
		if err != nil {
			logging.Fatal(err)
		}
	},
}

func init() {
	OperatorCmd.AddCommand(restartCmd)
	OperatorCmd.AddCommand(setCmd)
	OperatorCmd.AddCommand(logsCmd)
	logsCmd.Flags().BoolVarP(&logsFollow, "follow", "f", false, "specify if the logs should be streamed")
	logsCmd.Flags().BoolVarP(&logsPrevious, "previous", "p", false, "print the logs for the previous instance of the container if it crashed")
	logsCmd.Flags().BoolVar(&logsTimestamps, "timestamps", false, "include timestamps on each line in the log output")
	logsCmd.Flags().Int64Var(&logsTail, "tail", -1, "lines of recent log file to display, defaults to all")
	logsCmd.Flags().DurationVar(&logsSince, "since", 0, "only return logs newer than a relative duration like 5s, 2m, or 3h")
}

func validateLogsFlags(tail int64, since time.Duration) error {
	if tail < -1 {
		return fmt.Errorf("invalid --tail %d, must be greater than or equal to -1", tail)
	}
	if since < 0 {
		return fmt.Errorf("invalid --since %s, must not be negative", since)
	}

	return nil
}

func logOptions(follow, previous, timestamps bool, tail int64, since time.Duration) *corev1.PodLogOptions {
	opts := &corev1.PodLogOptions{
		Container:  "rook-ceph-operator",
		Follow:     follow,
		Previous:   previous,
		Timestamps: timestamps,
	}
	if tail >= 0 {
		opts.TailLines = &tail
	}
	if since > 0 {
		// round up: truncating a sub-second duration to 0 would mean "no limit" rather than "almost none"
		sinceSeconds := int64(math.Ceil(since.Seconds()))
		opts.SinceSeconds = &sinceSeconds
	}

	return opts
}
