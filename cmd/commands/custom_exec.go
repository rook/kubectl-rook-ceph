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

	"github.com/rook/kubectl-rook-ceph/pkg/filesystem"
	"github.com/spf13/cobra"
)

func addCustomExecFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().String("pod-name", "", "Pod to execute commands in")
	cmd.PersistentFlags().String("pod-namespace", "", "Namespace of the target pod")
	cmd.PersistentFlags().String("pod-container", "", "Container in the target pod")
	cmd.PersistentFlags().String("mon-ip", "", "Ceph monitor IP (e.g. 10.0.0.1:6789)")
	cmd.PersistentFlags().String("user-id", "", "Ceph user ID for authentication")
	cmd.PersistentFlags().String("user-key", "", "Ceph user key for authentication")
}

func parseCustomExecConfig(cmd *cobra.Command) (*filesystem.CustomExecConfig, error) {
	pod, _ := cmd.Flags().GetString("pod-name")
	if pod == "" {
		return nil, nil
	}
	ns, _ := cmd.Flags().GetString("pod-namespace")
	container, _ := cmd.Flags().GetString("pod-container")
	monIP, _ := cmd.Flags().GetString("mon-ip")
	userID, _ := cmd.Flags().GetString("user-id")
	userKey, _ := cmd.Flags().GetString("user-key")

	if ns == "" || container == "" || monIP == "" || userID == "" || userKey == "" {
		return nil, fmt.Errorf(
			"--pod-namespace, --pod-container, --mon-ip, --user-id, and --user-key are all required when --pod-name is set")
	}
	return &filesystem.CustomExecConfig{
		PodName:      pod,
		PodNamespace: ns,
		Container:    container,
		MonIP:        monIP,
		UserID:       userID,
		UserKey:      userKey,
	}, nil
}
