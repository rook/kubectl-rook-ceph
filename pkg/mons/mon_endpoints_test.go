/*
Copyright 2023 The Rook Authors. All rights reserved.

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
package mons

import (
	"context"
	"testing"

	"github.com/rook/kubectl-rook-ceph/pkg/k8sutil"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestParseMonEndpoint(t *testing.T) {
	ctx := context.TODO()

	newClient := fake.NewSimpleClientset
	k8s := newClient()
	clientsets := k8sutil.Clientsets{
		Kube: k8s,
	}
	ns := "rook-ceph"

	cm := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      MonConfigMap,
			Namespace: ns,
		},
		Data: map[string]string{
			"data": "10.96.52.53:6789",
		},
	}
	_, err := clientsets.Kube.CoreV1().ConfigMaps(ns).Create(ctx, cm, metav1.CreateOptions{})
	assert.NoError(t, err)

	monData := GetMonEndpoint(context.TODO(), clientsets.Kube, ns)
	assert.Equal(t, "10.96.52.53:6789", monData)
	assert.NotEqual(t, "10.96.52.54:6789", monData)

}
