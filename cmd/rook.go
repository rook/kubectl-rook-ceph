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
	"github.com/spf13/cobra"
)

var (
	KubeConfig           string
	RookNamespace        string
	CephClusterNamespace string
)

// rookCmd represents the rook command
var rootCmd = &cobra.Command{
	Use:   "rook",
	Short: "",
	Long:  ``,
	Args:  cobra.MinimumNArgs(1),
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	cobra.CheckErr(rootCmd.Execute())
}

func init() {

	//rootCmd.PersistentFlags().StringVar(cephCmd, "", "", "")
	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// rookCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// rookCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
	rootCmd.PersistentFlags().StringVar(&KubeConfig, "kubeconfig", "", "kubernetes config path")
	rootCmd.PersistentFlags().StringVar(&RookNamespace, "rook-namespace", "rook-ceph", "Kubernetes namespace where rook operator is running")
	rootCmd.PersistentFlags().StringVar(&CephClusterNamespace, "ceph-clsuter-namespace", "rook-ceph", "Kubernetes namespace where ceph cluster is created")
}
