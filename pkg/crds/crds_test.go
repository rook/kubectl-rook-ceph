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

package crds

import (
	"context"
	"fmt"
	"github.com/golang/mock/gomock"
	"github.com/rook/kubectl-rook-ceph/pkg/k8sutil"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"testing"
)

func NewUnstructuredData(version, kind, name string) *unstructured.Unstructured {
	obj := unstructured.Unstructured{}
	obj.SetAPIVersion(version)
	obj.SetKind(kind)
	obj.SetName(name)
	return &obj
}

func TestDeleteCustomResources(t *testing.T) {

	type MockListResourcesDynamically struct {
		items []unstructured.Unstructured
		err   error
	}

	type MockGetResourceDynamically struct {
		itemResource *unstructured.Unstructured
		err          error
	}

	type given struct {
		MockListResourcesDynamically  MockListResourcesDynamically
		MockGetResourceDynamically    MockGetResourceDynamically
		DeleteResourcesDynamicallyErr error
		PatchResourcesDynamicallyErr  error
	}

	type expected struct {
		err error
	}

	var cases = []struct {
		name     string
		given    given
		expected expected
	}{
		{
			name: "Should return error if was not able to run ListResourcesDynamically successfully",
			given: given{
				MockListResourcesDynamically: MockListResourcesDynamically{
					err: fmt.Errorf("error from ListResourcesDynamically"),
				},
			},
			expected: expected{
				err: fmt.Errorf("error from ListResourcesDynamically"),
			},
		},
		{
			name: "Should return no error if the error from ListResourcesDynamically was resource not found",
			given: given{
				MockListResourcesDynamically: MockListResourcesDynamically{
					err: errors.NewNotFound(schema.GroupResource{}, "cephrbdmirrors"),
				},
			},
		},
		{
			name: "Should return no error if no any error was thrown and the items from the list of resource is empty",
			given: given{
				MockListResourcesDynamically: MockListResourcesDynamically{},
			},
		},
		{
			name: "Should return error if an error was throw by DeleteResourcesDynamically",
			given: given{
				MockListResourcesDynamically: MockListResourcesDynamically{
					items: []unstructured.Unstructured{
						*NewUnstructuredData("v1", "cephblockpools", "cephblockpools"),
					},
				},
				DeleteResourcesDynamicallyErr: errors.NewNotFound(schema.GroupResource{}, "cephblockpools"),
			},
		},
		{
			name: "Should return no error if the error from GetResourcesDynamically was the server could not find the requested resource",
			given: given{
				MockListResourcesDynamically: MockListResourcesDynamically{
					items: []unstructured.Unstructured{
						*NewUnstructuredData("v1", "cephblockpools", "cephblockpools"),
					},
				},
				MockGetResourceDynamically: MockGetResourceDynamically{
					err: errors.NewNotFound(schema.GroupResource{}, "cephblockpools"),
				},
			},
		},
		{
			name: "Should return error if was unable to patch the resource kind cephblockpools",
			given: given{
				MockListResourcesDynamically: MockListResourcesDynamically{
					items: []unstructured.Unstructured{
						*NewUnstructuredData("v1", "cephblockpools", "cephblockpools"),
					},
				},
				MockGetResourceDynamically: MockGetResourceDynamically{
					itemResource: NewUnstructuredData("v1", "cephblockpools", "cephblockpools"),
				},
				PatchResourcesDynamicallyErr: fmt.Errorf("unable to patch the resource"),
			},
			expected: expected{
				err: fmt.Errorf("unable to patch the resource"),
			},
		},
		{
			name: "Should return error if was unable to patch the resource kind cephclusters with cleanupPolicy patch",
			given: given{
				MockListResourcesDynamically: MockListResourcesDynamically{
					items: []unstructured.Unstructured{
						*NewUnstructuredData("v1", "CephCluster", "cephclusters"),
					},
				},
				MockGetResourceDynamically: MockGetResourceDynamically{
					itemResource: NewUnstructuredData("v1", "CephCluster", "cephclusters"),
				},
				PatchResourcesDynamicallyErr: fmt.Errorf("unable to patch the resource"),
			},
			expected: expected{
				err: fmt.Errorf("unable to patch the resource"),
			},
		},
		{
			name: "Should return no error if was unable to patch the resource due the server could not find the requested resource",
			given: given{
				MockListResourcesDynamically: MockListResourcesDynamically{
					items: []unstructured.Unstructured{
						*NewUnstructuredData("v1", "CephCluster", "cephclusters"),
					},
				},
				MockGetResourceDynamically: MockGetResourceDynamically{
					itemResource: NewUnstructuredData("v1", "CephCluster", "cephclusters"),
				},
				PatchResourcesDynamicallyErr: errors.NewNotFound(schema.GroupResource{}, "cephclusters"),
			},
		},
		{
			name: "Should return no error if DeleteResourcesDynamically returns the server could not find the requested resource ",
			given: given{
				MockListResourcesDynamically: MockListResourcesDynamically{
					items: []unstructured.Unstructured{
						*NewUnstructuredData("v1", "CephCluster", "cephclusters"),
					},
				},
				MockGetResourceDynamically: MockGetResourceDynamically{
					itemResource: NewUnstructuredData("v1", "CephCluster", "cephclusters"),
				},
				DeleteResourcesDynamicallyErr: errors.NewNotFound(schema.GroupResource{}, "cephclusters"),
			},
		},
		{
			name: "Should return no error if the resource was deleted successfully",
			given: given{
				MockListResourcesDynamically: MockListResourcesDynamically{
					items: []unstructured.Unstructured{
						*NewUnstructuredData("v1", "CephCluster", "cephclusters"),
					},
				},
				MockGetResourceDynamically: MockGetResourceDynamically{
					itemResource: NewUnstructuredData("v1", "CephCluster", "cephclusters"),
				},
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			clientSets := k8sutil.NewMockClientsetsInterface(ctrl)
			clientSets.
				EXPECT().
				ListResourcesDynamically(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				Return(tc.given.MockListResourcesDynamically.items, tc.given.MockListResourcesDynamically.err).
				AnyTimes()

			clientSets.
				EXPECT().
				GetResourcesDynamically(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				Return(tc.given.MockGetResourceDynamically.itemResource, tc.given.MockGetResourceDynamically.err).
				AnyTimes()

			counter := 0

			clientSets.
				EXPECT().
				DeleteResourcesDynamically(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				Do(func(arg1, arg2, arg3, arg4, arg5, arg6 interface{}) error {
					if counter%2 == 1 {
						return nil
					}
					return tc.given.DeleteResourcesDynamicallyErr
				}).
				Return(tc.given.DeleteResourcesDynamicallyErr).
				AnyTimes()

			clientSets.
				EXPECT().
				PatchResourcesDynamically(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				Return(tc.given.PatchResourcesDynamicallyErr).
				AnyTimes()

			clusterNamespace := "rook-ceph"
			err := deleteCustomResources(context.Background(), clientSets, clusterNamespace)
			if tc.expected.err != nil {
				assert.Error(t, err)
				assert.Equal(t, tc.expected.err.Error(), err.Error())
				return
			}

			assert.NoError(t, err)
		})
	}
}
