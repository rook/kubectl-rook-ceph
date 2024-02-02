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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

func (c *Clientsets) ListResourcesDynamically(
	ctx context.Context,
	group string,
	version string,
	resource string,
	namespace string,
) ([]unstructured.Unstructured, error) {
	resourceId := schema.GroupVersionResource{
		Group:    group,
		Version:  version,
		Resource: resource,
	}

	list, err := c.Dynamic.Resource(resourceId).Namespace(namespace).
		List(ctx, metav1.ListOptions{})

	if err != nil {
		return nil, err
	}

	return list.Items, nil
}

func (c *Clientsets) DeleteResourcesDynamically(
	ctx context.Context,
	group string,
	version string,
	resource string,
	namespace string,
	resourceName string,
) error {

	resourceId := schema.GroupVersionResource{
		Group:    group,
		Version:  version,
		Resource: resource,
	}
	err := c.Dynamic.Resource(resourceId).Namespace(namespace).
		Delete(ctx, resourceName, metav1.DeleteOptions{})

	if err != nil {
		return err
	}
	return nil
}

func (c *Clientsets) PatchResourcesDynamically(
	ctx context.Context,
	group string,
	version string,
	resource string,
	namespace string,
	resourceName string,
	pt types.PatchType,
	data []byte,
) error {

	resourceId := schema.GroupVersionResource{
		Group:    group,
		Version:  version,
		Resource: resource,
	}

	_, err := c.Dynamic.Resource(resourceId).Namespace(namespace).
		Patch(ctx, resourceName, pt, data, metav1.PatchOptions{})

	if err != nil {
		return err
	}
	return nil
}

func (c *Clientsets) GetResourcesDynamically(
	ctx context.Context,
	group string,
	version string,
	resource string,
	name string,
	namespace string,
) (*unstructured.Unstructured, error) {
	resourceId := schema.GroupVersionResource{
		Group:    group,
		Version:  version,
		Resource: resource,
	}

	item, err := c.Dynamic.Resource(resourceId).Namespace(namespace).
		Get(ctx, name, metav1.GetOptions{})

	if err != nil {
		return nil, err
	}

	return item, nil
}

func (c *Clientsets) CreateResourcesDynamically(
	ctx context.Context,
	group string,
	version string,
	resource string,
	name *unstructured.Unstructured,
	namespace string,
) (*unstructured.Unstructured, error) {
	resourceId := schema.GroupVersionResource{
		Group:    group,
		Version:  version,
		Resource: resource,
	}

	item, err := c.Dynamic.Resource(resourceId).Namespace(namespace).
		Create(ctx, name, metav1.CreateOptions{})

	if err != nil {
		return nil, err
	}

	return item, nil
}
