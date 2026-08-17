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

package main

import (
	"context"
	"os/signal"
	"syscall"

	command "github.com/rook/kubectl-rook-ceph/cmd/commands"
	"github.com/rook/kubectl-rook-ceph/pkg/logging"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	addcommands()
	err := command.RootCmd.ExecuteContext(ctx)
	if err != nil {
		logging.Fatal(err)
	}
}

func addcommands() {
	command.RootCmd.AddCommand(
		command.CephCmd,
		command.MonCmd,
		command.RbdCmd,
		command.OperatorCmd,
		command.RookCmd,
		command.MaintenanceCmd,
		command.Health,
		command.DrCmd,
		command.RestoreCmd,
		command.DestroyClusterCmd,
		command.SubvolumeCmd,
		command.RadosCmd,
		command.FlattenRBDPVCCmd,
		command.RadosgwCmd,
		command.MultusCmd,
		command.CephFSSnapshotCmd,
		command.ToolboxCmd,
	)
}
