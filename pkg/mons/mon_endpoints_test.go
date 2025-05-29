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
	"fmt"
	"net"
	"testing"

	"github.com/rook/kubectl-rook-ceph/pkg/k8sutil"
	"github.com/stretchr/testify/assert"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestParseMonEndpoint(t *testing.T) {
	tests := []struct {
		name         string
		endpoint     string
		expectedHost string
		expectedPort string
		expectError  bool
	}{
		{
			name:         "Valid IPv4 endpoint",
			endpoint:     "192.168.1.1:6789",
			expectedHost: "192.168.1.1",
			expectedPort: "6789",
			expectError:  false,
		},
		{
			name:         "Valid IPv6 endpoint",
			endpoint:     "[2001:db8::1]:6789",
			expectedHost: "2001:db8::1",
			expectedPort: "6789",
			expectError:  false,
		},
		{
			name:         "Invalid endpoint - missing port",
			endpoint:     "192.168.1.1",
			expectedHost: "",
			expectedPort: "",
			expectError:  true,
		},
		{
			name:         "Invalid endpoint - invalid IPv4 address",
			endpoint:     "192.168.1.300:6789",
			expectedHost: "",
			expectedPort: "",
			expectError:  true,
		},
		{
			name:         "Invalid endpoint - invalid IPv6 format",
			endpoint:     "2001:db8::1:6789",
			expectedHost: "",
			expectedPort: "",
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, port, err := ParseMonEndpoint(tt.endpoint)

			if (err != nil) != tt.expectError {
				t.Errorf("ParseMonEndpoint() error = %v, expectError = %v", err, tt.expectError)
				return
			}

			if tt.expectError {
				return
			}

			if host != tt.expectedHost {
				t.Errorf("ParseMonEndpoint() host = %v, expected %v", host, tt.expectedHost)
			}
			if port != tt.expectedPort {
				t.Errorf("ParseMonEndpoint() port = %v, expected %v", port, tt.expectedPort)
			}
		})
	}
}

func TestGetMonEndpoint(t *testing.T) {
	ctx := context.TODO()

	tests := []struct {
		name     string
		data     string
		expected string
	}{
		{
			name:     "Legacy format IPv4",
			data:     "10.96.52.53:6789",
			expected: "10.96.52.53:6789",
		},
		{
			name:     "Single IPv4 monitor with name",
			data:     "mon-a=192.168.1.100:6789",
			expected: "192.168.1.100:6789",
		},
		{
			name:     "Single IPv6 monitor with name",
			data:     "i=[2a02:5501:31:c0a::4]:6789",
			expected: "[2a02:5501:31:c0a::4]:6789",
		},
		{
			name:     "Multiple IPv6 monitors",
			data:     "j=[2a02:5501:31:c0a::3]:6789,i=[2a02:5501:31:c0a::4]:6789",
			expected: "[2a02:5501:31:c0a::3]:6789,[2a02:5501:31:c0a::4]:6789",
		},
		{
			name:     "Mixed IPv4 and IPv6 monitors",
			data:     "mon-a=192.168.1.100:6789,i=[2a02:5501:31:c0a::4]:6789",
			expected: "192.168.1.100:6789,[2a02:5501:31:c0a::4]:6789",
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			newClient := fake.NewSimpleClientset
			k8s := newClient()
			clientsets := k8sutil.Clientsets{
				Kube: k8s,
			}

			ns := fmt.Sprintf("rook-ceph-test-%d", i)

			cm := &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      MonConfigMap,
					Namespace: ns,
				},
				Data: map[string]string{
					"data": tt.data,
				},
			}

			_, err := clientsets.Kube.CoreV1().ConfigMaps(ns).Create(ctx, cm, metav1.CreateOptions{})
			assert.NoError(t, err)

			monData := GetMonEndpoint(ctx, clientsets.Kube, ns)
			assert.Equal(t, tt.expected, monData)
		})
	}
}

func TestMonEndpointFormatting(t *testing.T) {
	tests := []struct {
		name         string
		goodMon      string
		ip           string
		port         string
		expectedData string
	}{
		{
			name:         "IPv4 address formatting",
			goodMon:      "mon-a",
			ip:           "192.168.1.100",
			port:         "6789",
			expectedData: "mon-a=192.168.1.100:6789",
		},
		{
			name:         "IPv6 address formatting",
			goodMon:      "i",
			ip:           "2a02:5501:31:c0a::4",
			port:         "6789",
			expectedData: "i=[2a02:5501:31:c0a::4]:6789",
		},
		{
			name:         "IPv6 localhost formatting",
			goodMon:      "mon-local",
			ip:           "::1",
			port:         "6789",
			expectedData: "mon-local=[::1]:6789",
		},
		{
			name:         "IPv4 localhost formatting",
			goodMon:      "mon-local",
			ip:           "127.0.0.1",
			port:         "6789",
			expectedData: "mon-local=127.0.0.1:6789",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fmt.Sprintf("%s=%s", tt.goodMon, net.JoinHostPort(tt.ip, tt.port))
			assert.Equal(t, tt.expectedData, result)
		})
	}
}
