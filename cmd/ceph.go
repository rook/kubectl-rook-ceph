/*
Copyright © 2021 kubectl-rook-ceph

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
package cmd

import (
	"kubectl-rook-ceph/pkg/ceph"

	"github.com/spf13/cobra"
)

var (
	format string
	file   string
	pool   string
)

// cephCmd represents the ceph command
var cephCmd = &cobra.Command{
	Use:   "ceph",
	Short: "",
	Long:  ``,
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ceph.CephCommands(cmd, args)
	},
}

func init() {
	rootCmd.AddCommand(cephCmd)
	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// cephCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// cephCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
	cephCmd.Flags().StringVarP(&format, "format", "f", "", "format json pretty output")
	cephCmd.Flags().StringVarP(&file, "o", "o", "", "file name")
	cephCmd.Flags().StringVarP(&pool, "pool", "", "", "pool name")
}
