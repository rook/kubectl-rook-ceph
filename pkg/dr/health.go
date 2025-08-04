package dr

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/rook/kubectl-rook-ceph/pkg/exec"
	"github.com/rook/kubectl-rook-ceph/pkg/k8sutil"
	"github.com/rook/kubectl-rook-ceph/pkg/logging"
	rookv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type secretData struct {
	Key      string `json:"key"`
	MonHost  string `json:"mon_host"`
	ClientId string `json:"client_id"`
}

func Health(ctx context.Context, clientsets *k8sutil.Clientsets, operatorNamespace, cephClusterNamespace string, args []string) {
	logging.Info("fetching the cephblockpools with mirroring enabled")
	blockPoolList, err := clientsets.Rook.CephV1().CephBlockPools(cephClusterNamespace).List(ctx, v1.ListOptions{})
	if err != nil {
		logging.Fatal(err)
	}

	var mirrorBlockPool rookv1.CephBlockPool
	for _, blockPool := range blockPoolList.Items {
		if blockPool.Spec.Mirroring.Enabled && blockPool.Spec.Mirroring.Peers != nil {
			mirrorBlockPool = blockPool
			logging.Info("found %q cephblockpool with mirroring enabled", mirrorBlockPool.Name)
		}
	}

	if mirrorBlockPool.Name == "" {
		logging.Warning("DR is not confiqured, cephblockpool with mirroring enabled not found.")
		return
	}

	var secretDetail string
	for _, data := range mirrorBlockPool.Spec.Mirroring.Peers.SecretNames {
		if data != "" {
			secretDetail = data
			break
		}
	}

	secretData, err := extractSecretData(ctx, clientsets.Kube, operatorNamespace, cephClusterNamespace, secretDetail)
	if err != nil {
		logging.Fatal(fmt.Errorf("failed to extract secret %s", secretDetail))
	}

	logging.Info("running ceph status from peer cluster")

	cephArgs := []string{"-s", "--mon-host", secretData.MonHost, "--id", secretData.ClientId, "--key", secretData.Key}

	if len(args) == 0 {
		cephArgs = append(cephArgs, "--debug-ms", "0")
	} else {
		cephArgs = append(cephArgs, args...)
	}

	cephStatus, err := exec.RunCommandInOperatorPod(ctx, clientsets, "ceph", cephArgs, operatorNamespace, cephClusterNamespace, true)
	if err != nil {
		logging.Warning("failed to get ceph status from peer cluster, please check for network issues between the clusters")
		return
	}
	logging.Info("%s", cephStatus)

	logging.Info("running mirroring daemon health")

	_, err = exec.RunCommandInOperatorPod(ctx, clientsets, "rbd", []string{"-p", mirrorBlockPool.Name, "mirror", "pool", "status"}, cephClusterNamespace, operatorNamespace, false)
	if err != nil {
		logging.Error(err)
	}
}

func extractSecretData(ctx context.Context, k8sclientset kubernetes.Interface, operatorNamespace, cephClusterNamespace, secretName string) (*secretData, error) {
	secret, err := k8sclientset.CoreV1().Secrets(cephClusterNamespace).Get(ctx, secretName, v1.GetOptions{})
	if err != nil {
		return nil, err
	}
	decodeSecretData, err := base64.StdEncoding.DecodeString(string(secret.Data["token"]))
	if err != nil {
		return nil, err
	}

	var data *secretData
	err = json.Unmarshal(decodeSecretData, &data)
	if err != nil {
		return nil, err
	}
	return data, nil
}
