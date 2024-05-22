package flatten_rbd_pvc

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/rook/kubectl-rook-ceph/pkg/exec"
	"github.com/rook/kubectl-rook-ceph/pkg/k8sutil"
	"github.com/rook/kubectl-rook-ceph/pkg/logging"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type RBDInfoOutput struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Watcher struct {
	Address string `json:"address"`
}

type RBDStatusOutput struct {
	Watchers []Watcher `json:"watchers"`
}

func FlattenRBDPVC(ctx context.Context, clientSets *k8sutil.Clientsets, operatorNamespace, clusterNamespace, namespace, pvcName string, allowInUse bool) {
	pvc, err := clientSets.Kube.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, pvcName, metav1.GetOptions{})
	if err != nil {
		logging.Fatal(err, "failed to get PVC %s/%s", namespace, pvcName)
	}
	if pvc.DeletionTimestamp != nil {
		logging.Fatal(fmt.Errorf("PVC %s is deleting", pvcName))
	}
	if pvc.Status.Phase != corev1.ClaimBound {
		logging.Fatal(fmt.Errorf("PVC %s is not bound", pvcName))
	}

	shouldDeleteTempImage := false
	if pvc.Spec.DataSource != nil {
		switch pvc.Spec.DataSource.Kind {
		case "PersistentVolumeClaim":
			shouldDeleteTempImage = true
		case "VolumeSnapshot":
		default:
			logging.Fatal(fmt.Errorf("PVC %s is not a cloned image", pvcName))
		}
	}

	pvName := pvc.Spec.VolumeName
	pv, err := clientSets.Kube.CoreV1().PersistentVolumes().Get(ctx, pvName, metav1.GetOptions{})
	if err != nil {
		logging.Fatal(fmt.Errorf("failed to get PV %s", pvName))
	}
	imageName, ok := pv.Spec.CSI.VolumeAttributes["imageName"]
	if !ok {
		logging.Fatal(fmt.Errorf("PV %s doesn't contains `imageName` in VolumeAttributes", pvName))
	}
	poolName, ok := pv.Spec.CSI.VolumeAttributes["pool"]
	if !ok {
		logging.Fatal(fmt.Errorf("PV %s doesn't contains `pool` in VolumeAttributes", pvName))
	}

	if !allowInUse {
		out, err := exec.RunCommandInOperatorPod(ctx, clientSets, "rbd", []string{"-p", poolName, "status", imageName, "--format=json"}, operatorNamespace, clusterNamespace, false)
		if err != nil {
			logging.Fatal(fmt.Errorf("failed to stat %s/%s", poolName, imageName))
		}
		var status RBDStatusOutput
		json.Unmarshal([]byte(out), &status)
		if len(status.Watchers) > 0 {
			logging.Fatal(fmt.Errorf("flatten in-use pvc %s is not allowed. If you want to do, run with `--allow-in-use` option", pvcName))
		}
	}

	if shouldDeleteTempImage {
		deleteTempImage(ctx, clientSets, operatorNamespace, clusterNamespace, poolName, imageName)
	}
	logging.Info("flattening the target RBD image %s/%s", poolName, imageName)
	_, err = exec.RunCommandInOperatorPod(ctx, clientSets, "ceph", []string{"rbd", "task", "add", "flatten", fmt.Sprintf("%s/%s", poolName, imageName)}, operatorNamespace, clusterNamespace, false)
	if err != nil {
		logging.Fatal(fmt.Errorf("failed to flatten %s/%s", poolName, imageName))
	}
}

func deleteTempImage(ctx context.Context, clientSets *k8sutil.Clientsets, operatorNamespace, cephClusterNamespace, poolName, imageName string) {
	tempImageName := imageName + "-temp"

	out, err := exec.RunCommandInOperatorPod(ctx, clientSets, "rbd", []string{"-p", poolName, "info", "--format=json", tempImageName}, operatorNamespace, cephClusterNamespace, false)
	if err != nil {
		logging.Error(fmt.Errorf("failed to run `rbd info` for rbd image %s/%s", poolName, tempImageName))
		return
	}
	var info RBDInfoOutput
	json.Unmarshal([]byte(out), &info)
	id := info.ID
	logging.Info("removing the temporary RBD image %s/%s if exist", poolName, tempImageName)
	_, err = exec.RunCommandInOperatorPod(ctx, clientSets, "rbd", []string{"-p", poolName, "trash", "mv", tempImageName}, operatorNamespace, cephClusterNamespace, false)
	if err != nil {
		logging.Fatal(fmt.Errorf("failed to move rbd image %s/%s to trash", poolName, tempImageName))
	}
	if id != "" {
		_, err = exec.RunCommandInOperatorPod(ctx, clientSets, "ceph", []string{"rbd", "task", "add", "trash", "remove", fmt.Sprintf("%s/%s", poolName, id)}, operatorNamespace, cephClusterNamespace, false)
		if err != nil {
			logging.Fatal(fmt.Errorf("failed to create a task to remove %s/%s from trash", poolName, id))
		}
	}
}
