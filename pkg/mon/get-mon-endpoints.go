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
package mon

import (
	"context"
	"fmt"

	"encoding/json"

	"kubectl-rook-ceph/pkg"

	log "github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func GetMonEndpoints() {
	// Get Kubernetes Client
	_, client := pkg.GetKubeClinet()

	cm, err := client.ConfigMaps("rook-ceph").Get(context.TODO(), "rook-ceph-mon-endpoints", metav1.GetOptions{})
	if err != nil {
		log.Errorln("failed to get confimap rook-ceph-mon-endpoints")
	}

	data, err := json.MarshalIndent(cm.Data, "", "    ")
	if err != nil {
		log.Errorln("failed to marshal")
	}
	fmt.Println("Mon-endpoints")
	fmt.Println(string(data))
}
