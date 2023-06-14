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

package command

import "testing"

func Test_trimGoVersionFromRookVersion(t *testing.T) {
	type args struct {
		rookVersion string
	}

	sampleRookVersion := `rook: v1.11.0-alpha.0.420.gd9a17691c
go: go1.19.10s`
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "trim go version from rook version",
			args: args{rookVersion: sampleRookVersion},
			want: "rook: v1.11.0-alpha.0.420.gd9a17691c",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := trimGoVersionFromRookVersion(tt.args.rookVersion); got != tt.want {
				t.Errorf("trimGoVersionFromRookVersion() = %v, want %v", got, tt.want)
			}
		})
	}
}
