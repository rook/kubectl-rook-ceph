#!/usr/bin/env bash

# Copyright 2021 The Rook Authors. All rights reserved.
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

set -eEuo pipefail

usages() {
  cat <<EOF
Description:
  'kubectl rook-ceph ceph' provides common management and troubleshooting tools for Ceph.

Commands:
  ceph - run any 'ceph' CLI command as given
    args  - any 'ceph' CLI argument or flag

Usage:
  kubectl rook-ceph ceph [command] [args...]

EOF
}

ceph_command() {
  kubectl --namespace rook-ceph exec deploy/rook-ceph-operator -- /bin/bash -c "${*} --conf=/var/lib/rook/rook-ceph/rook-ceph.config"
}

main() {
  if [ $# -ge 2 ]; then
    if [ "$1" = "ceph" ]; then
      ceph_command "$@"
    fi
  else
    usages
  fi
}

main "$@"
