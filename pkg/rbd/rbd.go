package rbd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/rook/kubectl-rook-ceph/pkg/exec"
	"github.com/rook/kubectl-rook-ceph/pkg/k8sutil"
	"github.com/rook/kubectl-rook-ceph/pkg/logging"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type poolData struct {
	imageList     []string
	namespaceList []string
}

// ListImages retrieves and displays Ceph block pools and their associated images.
func ListImages(ctx context.Context, clientsets *k8sutil.Clientsets, operatorNamespace, clusterNamespace string) {
	blockPoolNames := fetchBlockPools(ctx, clientsets, clusterNamespace)
	blockPoolNames = fetchBlockPoolNamespaces(ctx, clientsets, clusterNamespace, blockPoolNames)
	retrieveRBDImages(ctx, clientsets, operatorNamespace, clusterNamespace, blockPoolNames)
	printBlockPoolNames(blockPoolNames)
}

// fetchBlockPools retrieves the list of CephBlockPools and initializes the poolData map.
func fetchBlockPools(ctx context.Context, clientsets *k8sutil.Clientsets, clusterNamespace string) map[string]poolData {
	blockPoolList, err := clientsets.Rook.CephV1().CephBlockPools(clusterNamespace).List(ctx, v1.ListOptions{})
	if err != nil {
		logging.Fatal(fmt.Errorf("failed to list CephBlockPools: %w", err))
	}
	blockPoolNames := make(map[string]poolData)
	for _, blockPool := range blockPoolList.Items {
		if blockPool.Name == "builtin-mgr" {
			continue
		}
		blockPoolNames[blockPool.Name] = poolData{
			imageList:     []string{},
			namespaceList: []string{},
		}
	}
	return blockPoolNames
}

// fetchBlockPoolNamespaces retrieves the CephBlockPoolRadosNamespaces and associates them with pools.
func fetchBlockPoolNamespaces(ctx context.Context, clientsets *k8sutil.Clientsets, clusterNamespace string, blockPoolNames map[string]poolData) map[string]poolData {
	blockPoolNamespaceList, err := clientsets.Rook.CephV1().CephBlockPoolRadosNamespaces(clusterNamespace).List(ctx, v1.ListOptions{})
	if err != nil {
		logging.Fatal(fmt.Errorf("failed to list CephBlockPoolRadosNamespaces: %w", err))
	}

	for _, blockPoolNamespace := range blockPoolNamespaceList.Items {
		name := blockPoolNamespace.Spec.Name
		if name == "" {
			name = blockPoolNamespace.ObjectMeta.Name
		}

		if poolInfo, exists := blockPoolNames[blockPoolNamespace.Spec.BlockPoolName]; exists {
			poolInfo.namespaceList = append(poolInfo.namespaceList, name)
			blockPoolNames[blockPoolNamespace.Spec.BlockPoolName] = poolInfo
		}
	}
	return blockPoolNames
}

// retrieveRBDImages fetches RBD images for each pool and namespace.
func retrieveRBDImages(ctx context.Context, clientsets *k8sutil.Clientsets, operatorNamespace, clusterNamespace string, blockPoolNames map[string]poolData) {
	for poolName, poolInfo := range blockPoolNames {
		if len(poolInfo.namespaceList) == 0 {
			images, err := getRBDImages(ctx, clientsets, poolName, "", operatorNamespace, clusterNamespace)
			if err != nil {
				logging.Fatal(fmt.Errorf("failed to list images for pool %s: %w", poolName, err))
			}
			poolInfo.imageList = images
			blockPoolNames[poolName] = poolInfo
		} else {
			for _, namespace := range poolInfo.namespaceList {
				images, err := getRBDImages(ctx, clientsets, poolName, namespace, operatorNamespace, clusterNamespace)
				if err != nil {
					logging.Fatal(fmt.Errorf("failed to list images for pool %s and namespace %s: %w", poolName, namespace, err))
				}
				poolInfo.imageList = append(poolInfo.imageList, images...)
				blockPoolNames[poolName] = poolInfo
			}
		}
	}
}

// getRBDImages runs the RBD command to fetch images for a pool and namespace.
func getRBDImages(ctx context.Context, clientsets *k8sutil.Clientsets, poolName, namespace, operatorNamespace, clusterNamespace string) ([]string, error) {
	cmd := "rbd"
	args := []string{"ls", "--pool=" + poolName}
	if namespace != "" {
		args = append(args, "--namespace="+namespace)
	}
	output, err := exec.RunCommandInOperatorPod(ctx, clientsets, cmd, args, operatorNamespace, clusterNamespace, true)
	if err != nil {
		return nil, fmt.Errorf("failed to list RBD images: %w", err)
	}
	// Check if the output is empty
	if strings.TrimSpace(output) == "" {
		return []string{"---"}, nil
	}
	return strings.Fields(output), nil
}

// printBlockPoolNames prints the block pools and their associated images in a tabular format.
func printBlockPoolNames(blockPoolNames map[string]poolData) {
	writer := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	defer writer.Flush()

	fmt.Fprintln(writer, "poolName\timageName\tnamespace\t")
	fmt.Fprintln(writer, "--------\t---------\t---------\t")
	for poolName, poolInfo := range blockPoolNames {
		if len(poolInfo.namespaceList) == 0 {
			for _, image := range poolInfo.imageList {
				fmt.Fprintf(writer, "%s\t%s\t---\t\n", poolName, image)
			}
		} else {
			for i := 0; i < len(poolInfo.namespaceList); i++ {
				fmt.Fprintf(writer, "%s\t%s\t%s\t\n", poolName, poolInfo.imageList[i], poolInfo.namespaceList[i])
			}
		}
	}
}
