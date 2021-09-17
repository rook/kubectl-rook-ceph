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
package operator

import (
	"context"
	"fmt"
	"kubectl-rook-ceph/pkg"

	log "github.com/sirupsen/logrus"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func RestartOperatorPod(args []string) {
	// Get Kubernetes Client
	clientset, client := pkg.GetKubeClinet()

	opts := metav1.ListOptions{LabelSelector: fmt.Sprintf("app=%s", "rook-ceph-operator")}
	list, err := client.Pods("rook-ceph").List(context.TODO(), opts)
	if err != nil || len(list.Items) == 0 {
		log.Error("failed to get rook-ceph-operator pod")
		log.Fatal(err)
	}

	if len(args) == 1 && args[0] == "restart" {
		err = clientset.CoreV1().Pods("rook-ceph").Delete(context.TODO(), list.Items[0].Name, metav1.DeleteOptions{})
		if err != nil {
			log.Error("failed to delete rook-ceph-operator pod")
			log.Fatal(err)
		}
		log.Info("Successfully restarted rook-ceph-operator pod")
	} else {
		log.Errorln("Not a valid operator argument")
	}

}
