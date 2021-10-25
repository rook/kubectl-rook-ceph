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
  if [ $# -ge 2 ]; then
    kubectl --namespace "$2" exec deploy/rook-ceph-operator -- /bin/bash -c "$1 --conf=/var/lib/rook/$2/$2.config"
  else
    kubectl --namespace rook-ceph exec deploy/rook-ceph-operator -- /bin/bash -c "$1 --conf=/var/lib/rook/rook-ceph/rook-ceph.config"
  fi
}

main() {
  if [ $# -ge 2 ]; then
    if [[ $* =~ "ceph" ]] && [[ $* =~ "-n"  || $* =~ "--namespace" ]];  then
      index=0
      # This loop is interating arguments to read namespace flag and it's value
      for val in "$@"; do
        ((++index))
        if [[ $val == "-n" ]] || [[ $val == "--namespace" ]]; then
          remove_namespace_arg_from_command="${*:$index:2}"
          namespace="${*:$index+1:1}"
          break
        fi
      done

      # shellcheck disable=SC2001 # not without sed
      command=$(echo "$*" | sed s/"$remove_namespace_arg_from_command"//)
      ceph_command "$command" "$namespace"
    elif [ "$1" = "ceph" ]; then
      command="$*"
      ceph_command "$command"
    fi
  else
    usages
  fi
}

main "$@"
