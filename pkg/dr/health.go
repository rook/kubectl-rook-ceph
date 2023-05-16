package dr

import (
	ctx "context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	log "github.com/sirupsen/logrus"

	"github.com/rook/kubectl-rook-ceph/pkg/exec"
	"github.com/rook/kubectl-rook-ceph/pkg/k8sutil"
	rookv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type secretData struct {
	Key      string `json:"key"`
	MonHost  string `json:"mon_host"`
	ClientId string `json:"client_id"`
}

func Health(context *k8sutil.Context, operatorNamespace, cephClusterNamespace string, args []string) {
	fmt.Println("INFO: fetching the cephblockpools with mirroring enabled")
	blockPoolList, err := context.RookClientset.CephV1().CephBlockPools(cephClusterNamespace).List(ctx.TODO(), v1.ListOptions{})
	if err != nil {
		log.Error(err)
	}

	var mirrorBlockPool rookv1.CephBlockPool
	for _, blockPool := range blockPoolList.Items {
		if blockPool.Spec.Mirroring.Enabled && blockPool.Spec.Mirroring.Peers != nil {
			mirrorBlockPool = blockPool
			fmt.Printf("found %q cephblockpool with mirroring enabled\n", mirrorBlockPool.Name)
		}
	}

	if mirrorBlockPool.Name == "" {
		fmt.Printf("DR is not confiqured, cephblockpool with mirroring enabled not found.\n")
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
		fmt.Printf("failed to extract secret %s", secretDetail)
		log.Fatal(err)
	}

	fmt.Println("running ceph status from peer cluster")

	cephArgs := []string{"-s", "--mon-host", secretData.MonHost, "--id", secretData.ClientId, "--key", secretData.Key}

	if len(args) == 0 {
		cephArgs = append(cephArgs, "--debug-ms", "0")
	} else {
		cephArgs = append(cephArgs, args...)
	}

	cephStatus := exec.RunCommandInOperatorPod(context, "ceph", cephArgs, operatorNamespace, cephClusterNamespace, false)
	if cephStatus == "" {
		fmt.Println("failed to get ceph status from peer cluster, please check for network issues between the clusters")
		return
	}
	fmt.Println(cephStatus)

	fmt.Println("running mirroring daemon health")

	fmt.Println(exec.RunCommandInOperatorPod(context, "rbd", []string{"-p", mirrorBlockPool.Name, "mirror", "pool", "status"}, cephClusterNamespace, operatorNamespace, true))
}

func extractSecretData(context *k8sutil.Context, operatorNamespace, cephClusterNamespace, secretName string) (*secretData, error) {
	secret, err := context.Clientset.CoreV1().Secrets(cephClusterNamespace).Get(ctx.TODO(), secretName, v1.GetOptions{})
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
