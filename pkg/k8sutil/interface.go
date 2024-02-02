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

package k8sutil

import (
	"context"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
)

//go:generate mockgen -package=k8sutil --build_flags=--mod=mod -destination=mocks.go github.com/rook/kubectl-rook-ceph/pkg/k8sutil ClientsetsInterface
type ClientsetsInterface interface {
	CreateResourcesDynamically(ctx context.Context, group string, version string, resource string, name *unstructured.Unstructured, namespace string) (*unstructured.Unstructured, error)
	ListResourcesDynamically(ctx context.Context, group string, version string, resource string, namespace string) ([]unstructured.Unstructured, error)
	GetResourcesDynamically(ctx context.Context, group string, version string, resource string, name string, namespace string) (*unstructured.Unstructured, error)
	DeleteResourcesDynamically(ctx context.Context, group string, version string, resource string, namespace string, resourceName string) error
	PatchResourcesDynamically(ctx context.Context, group string, version string, resource string, namespace string, resourceName string, pt types.PatchType, data []byte) error
}
