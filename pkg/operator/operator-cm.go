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
	"encoding/json"
	"fmt"
	"kubectl-rook-ceph/pkg"

	log "github.com/sirupsen/logrus"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func EditConfigMap(args []string) {
	// Get Kubernetes Client
	_, client := pkg.GetKubeClinet()
	cm, err := client.ConfigMaps("rook-ceph").Get(context.TODO(), "rook-ceph-operator-config", metav1.GetOptions{})
	if err != nil {
		log.Errorln("failed to get confimap rook-ceph-operator-config")
	}

	// update configMap
	cm.Data[args[1]] = args[2]

	cm, err = client.ConfigMaps("rook-ceph").Update(context.TODO(), cm, metav1.UpdateOptions{})
	if err != nil {
		log.Errorln("failed to update confimap rook-ceph-operator-config")
	}
	data, err := json.MarshalIndent(cm.Data, "", "    ")
	if err != nil {
		log.Errorln("failed to marshal")
	}

	fmt.Println("Updated configmap rook-ceph-operator-config data")
	fmt.Println("\n", string(data))
}
