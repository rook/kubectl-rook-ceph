# Copyright 2023 The Rook Authors. All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.


build:
	gofmt -w $(shell find . -type f -name '*.go')
	@echo
	env GOOS=$(shell go env GOOS) GOARCH=$(shell go env GOARCH) go build -o bin/kubectl-rook-ceph  cmd/main.go

clean:
	@rm -f bin/kubectl-rook-ceph

test:
	@echo "running unit tests"
	go test ./...

help :
	@echo "build : Create go binary."
	@echo "test  : Runs unit tests"
	@echo "clean : Remove go binary file."
