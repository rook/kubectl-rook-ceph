/*
Copyright 2024 The Rook Authors. All rights reserved.

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

package subvolume

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetOmapVal(t *testing.T) {

	tests := []struct {
		name     string
		val      string
		subvolid string
	}{
		{
			name:     "csi-vol-427774b4-340b-11ed-8d66-0242ac110005",
			val:      "csi.volume.427774b4-340b-11ed-8d66-0242ac110005",
			subvolid: "427774b4-340b-11ed-8d66-0242ac110005",
		},
		{
			name:     "nfs-export-427774b4-340b-11ed-8d66-0242ac110005",
			val:      "csi.volume.427774b4-340b-11ed-8d66-0242ac110005",
			subvolid: "427774b4-340b-11ed-8d66-0242ac110005",
		},
		{
			name:     "",
			val:      "",
			subvolid: "",
		},
		{
			name:     "csi-427774b4-340b-11ed-8d66-0242ac11000",
			val:      "csi.volume.340b-11ed-8d66-0242ac11000",
			subvolid: "340b-11ed-8d66-0242ac11000",
		},
		{
			name:     "csi-427774b440b11ed8d660242ac11000",
			val:      "",
			subvolid: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if val, subvolid := getOmapVal(tt.name); val != tt.val && subvolid != tt.subvolid {
				t.Errorf("getOmapVal()= got val %v, want val %v,got subvolid %v want subvolid %v", val, tt.val, subvolid, tt.subvolid)
			}
		})
	}
}

func TestGenerateSubvolumeNameFromVolumeHandle(t *testing.T) {

	tests := []struct {
		prefix       string
		volumeHandle string
		name         string
		err          error
	}{
		{
			prefix:       "myvol-",
			volumeHandle: "0001-0011-openshift-storage-0000000000000001-aac40941-9b54-432f-8a63-3b1614a4e024",
			name:         "myvol-aac40941-9b54-432f-8a63-3b1614a4e024",
		},
		{
			volumeHandle: "0001-0011-openshift-storage-0000000000000001-aac40941-9b54-432f-8a63-3b1614a4e024",
			name:         "csi-vol-aac40941-9b54-432f-8a63-3b1614a4e024",
		},
		{
			volumeHandle: "0001-0009-rook-ceph-0000000000000001-aac40941-9b54-432f-8a63-3b1614a4e024",
			name:         "csi-vol-aac40941-9b54-432f-8a63-3b1614a4e024",
		},
		{
			prefix:       "",
			volumeHandle: "0002-0009-rook-ceph-0000000000000001-aac40941-9b54-432f-8a63-3b1614a4e024",
			err:          fmt.Errorf("failed to extract prefix: volume handle \"0002-0009-rook-ceph-0000000000000001-aac40941-9b54-432f-8a63-3b1614a4e024\" uses an unsupported version format"),
		},
		{
			volumeHandle: "ac40941-9b54-432f-8a63-3b1614a4e024",
			err:          fmt.Errorf("volume handle too short to extract subvolume name: ac40941-9b54-432f-8a63-3b1614a4e024"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.volumeHandle, func(t *testing.T) {
			name, err := generateSubvolumeNameFromVolumeHandle(tt.prefix, tt.volumeHandle)
			if err != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.err.Error(), err.Error())
				return
			}
			assert.Equal(t, name, tt.name)
		})
	}
}

func TestGetSnapOmapVal(t *testing.T) {

	tests := []struct {
		name   string
		val    string
		snapid string
	}{
		{
			name:   "csi-snap-427774b4-340b-11ed-8d66-0242ac110005",
			val:    "csi.snap.427774b4-340b-11ed-8d66-0242ac110005",
			snapid: "427774b4-340b-11ed-8d66-0242ac110005",
		},
		{
			name:   "",
			val:    "",
			snapid: "",
		},
		{
			name:   "csi-427774b4-340b-11ed-8d66-0242ac11000",
			val:    "csi.snap.340b-11ed-8d66-0242ac11000",
			snapid: "340b-11ed-8d66-0242ac11000",
		},
		{
			name:   "csi-427774b440b11ed8d660242ac11000",
			val:    "",
			snapid: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if val, snapid := getSnapOmapVal(tt.name); val != tt.val && snapid != tt.snapid {
				t.Errorf("getSnapOmapVal()= got val %v, want val %v,got snapid %v want snapid %v", val, tt.val, snapid, tt.snapid)
			}
		})
	}
}

func TestGetSnapshotHandleId(t *testing.T) {

	tests := []struct {
		name string
		val  string
	}{
		{
			name: "0001-0009-rook-ceph-0000000000000001-17b95621-58e8-4676-bc6a-39e928f19d23",
			val:  "17b95621-58e8-4676-bc6a-39e928f19d23",
		},
		{
			name: "",
			val:  "",
		},
		{
			name: "0001-0009-rook-0000000000000001-17b95621-58e8-4676-bc6a-39e928f19d23",
			val:  "58e8-4676-bc6a-39e928f19d23",
		},
		{
			name: "rook-427774b440b11ed8d660242ac11000",
			val:  "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if val := getSnapshotHandleId(tt.name); val != tt.val {
				assert.Equal(t, val, tt.val)
			}
		})
	}
}
