package dr

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/rook/kubectl-rook-ceph/pkg/exec"
	"github.com/rook/kubectl-rook-ceph/pkg/k8sutil"
	"github.com/rook/kubectl-rook-ceph/pkg/logging"
	rookv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type secretData struct {
	Key      string `json:"key"`
	MonHost  string `json:"mon_host"`
	ClientId string `json:"client_id"`
}

func Health(context *k8sutil.Context, operatorNamespace, cephClusterNamespace string, args []string) {
	logging.Info("fetching the cephblockpools with mirroring enabled")
	blockPoolList, err := context.RookClientset.CephV1().CephBlockPools(cephClusterNamespace).List(context.Context, v1.ListOptions{})
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

	secretData, err := extractSecretData(context, operatorNamespace, cephClusterNamespace, secretDetail)
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

	cephStatus := exec.RunCommandInOperatorPod(context, "ceph", cephArgs, operatorNamespace, cephClusterNamespace, false)
	if cephStatus == "" {
		logging.Warning("failed to get ceph status from peer cluster, please check for network issues between the clusters")
		return
	}
	logging.Info(cephStatus)

	logging.Info("running mirroring daemon health")

	fmt.Println(exec.RunCommandInOperatorPod(context, "rbd", []string{"-p", mirrorBlockPool.Name, "mirror", "pool", "status"}, cephClusterNamespace, operatorNamespace, true))
}

func extractSecretData(context *k8sutil.Context, operatorNamespace, cephClusterNamespace, secretName string) (*secretData, error) {
	secret, err := context.Clientset.CoreV1().Secrets(cephClusterNamespace).Get(context.Context, secretName, v1.GetOptions{})
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
