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

import "testing"

func TestGetOmapVal(t *testing.T) {

	tests := []struct {
		name string
		want string
	}{
		{
			name: "csi-vol-427774b4-340b-11ed-8d66-0242ac110005",
			want: "csi.volume.427774b4-340b-11ed-8d66-0242ac110005",
		},
		{
			name: "nfs-export-427774b4-340b-11ed-8d66-0242ac110005",
			want: "csi.volume.427774b4-340b-11ed-8d66-0242ac110005",
		},
		{
			name: "",
			want: "",
		},
		{
			name: "csi-427774b4-340b-11ed-8d66-0242ac11000",
			want: "csi.volume.340b-11ed-8d66-0242ac11000",
		},
		{
			name: "csi-427774b440b11ed8d660242ac11000",
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getOmapVal(tt.name); got != tt.want {
				t.Errorf("getOmapVal() = %v, want %v", got, tt.want)
			}
		})
	}
}
