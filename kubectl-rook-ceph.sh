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

####################################################################################################
# HELPER FUNCTIONS
####################################################################################################

function print_usage() {
  echo ""
  echo "DESCRIPTION"
  echo "kubectl rook-ceph provides common management and troubleshooting tools for Ceph."
  echo "USAGE"
  echo "  kubectl rook-ceph <main args> <command> <command args>"
  echo "MAIN ARGS"
  echo "  -h, --help                                : output help"
  echo "  -n, --namespace='rook-ceph'               : the namespace of the CephCluster"
  echo "  -o, --operator-namespace='rook-ceph'      : the namespace of the rook operator"
  echo " --context=<context_name>                   : the name of the Kubernetes context to be used"

  echo "COMMANDS"
  echo "  ceph <args>                               : call a 'ceph' CLI command with arbitrary args"
  echo "  rbd <args>                                : call a 'rbd' CLI command with arbitrary args"
  echo "  health                                    : check health of the cluster and common configuration issues"
  echo "  operator"
  echo "    restart                                 : restart the Rook-Ceph operator"
  echo "    set <property> <value>                  : Set the property in the rook-ceph-operator-config configmap."
  echo "  mons                                      : output mon endpoints"
  echo "    restore-quorum <mon-name>               : When quorum is lost, restore quorum to the remaining healthy mon"
  echo "  rook"
  echo "    version                                 : print the version of Rook"
  echo "    status                                  : print the phase and conditions of the CephCluster CR"
  echo "    status all                              : print the phase and conditions of all CRs"
  echo "    status <CR>                             : print the phase and conditions of CRs of a specific type, such as 'cephobjectstore', 'cephfilesystem', etc"
  echo "    purge-osd <osd-id> [--force]            : Permanently remove an OSD from the cluster. Multiple OSDs can be removed with a comma-separated list of IDs."
  echo "  debug                                     : Debug a deployment by scaling it down and creating a debug copy. This is supported for mons and OSDs only"
  echo "    start  <deployment-name>
           [--alternate-image <alternate-image>]    : Start debugging a deployment with an optional container image"
  echo "    stop <deployment-name>                  : Stop debugging a deployment"
  echo "  dr"
  echo "    health [debug args]                     : Print the ceph status of a peer cluster in a mirroring-enabled cluster. Optional ceph args such as: '--debug-ms 1'."
  echo ""
}

function fail_error() {
  print_usage >&2
  error_msg " $*" >&2
  exit 1
}

# return failure if the input is not a flag
function is_flag() {
  [[ "$1" == -* ]]
}

# return failure if the input (a flag value) doesn't exist
function val_exists() {
  local val="$1"
  [[ -n "$val" ]]
}

# fail with an error if the value is set
function flag_no_value() {
  local flag="$1"
  local value="$2"
  val_exists "$value" && fail_error "Flag '$flag' does not take a value"
}

# Usage: parse_flags 'set_value_function' "$@"
#
# This is a reusable function that will parse flags from the beginning of the "$@" (arguments) input
# until a non-flag argument is reached. It then returns the remaining arguments in a global env var
# called REMAINING_ARGS. For each parsed flag, it calls the user-specified callback function
# 'set_value_function' to set a config value.
#
# When a flag is reached, calls 'set_value_function' with the parsed flag and value as args 1 and 2.
# The 'set_value_function' must take 2 args in this order: flag, value
# The 'set_value_function' must return non-zero if the flag needs a value and was not given one.
#   Can copy-paste this line to achieve the above:  val_exists "$val" || return 1 # val should exist
# The 'set_value_function' must return zero in all other cases.
# The 'set_value_function' should call 'fail_error' if a flag is specified incorrectly.
# The 'set_value_function' should enforce flags that should have no values (use 'flag_no_value').
# The 'set_value_function' should record the config specified by the flag/value if it is valid.
# When a non-flag arg is reached, stop parsing and return the remaining args in REMAINING_ARGS.
REMAINING_ARGS=()
function parse_flags() {
  REMAINING_ARGS=()
  local set_value_function="$1"
  shift # pop set_value_function arg from the arg list
  while (($#)); do
    arg="$1"
    shift
    FLAG=""
    VAL=""
    case "$arg" in
    --*=*)              # long flag with a value, e.g., '--namespace=my-ns'
      FLAG="${arg%%=*}" # left of first equal
      VAL="${arg#*=}"   # right of first equal
      val_exists "$VAL" || fail_error "Flag '$FLAG' does not specify a value"
      ;;
    --*) # long flag without a value, e.g., '--help' or '--namespace my-ns'
      FLAG="$arg"
      VAL=""
      ;;
    -*)                              # short flags
      if [[ "${#arg}" -eq 2 ]]; then # short flag without a value, e.g., '-h' or '-n my-ns'
        FLAG="$arg"
        VAL=""
      else                     # short flag with a value, e.g., '-nmy-ns', or '-n=my-ns'
        FLAG="${arg:0:2}"      # first 2 chars
        VAL="${arg:2:${#arg}}" # remaining chars
        VAL="${VAL#*=}"        # strip first equal from the value
      fi
      ;;
    *)
      # This is not a flag, so stop parsing and return the stored remaining args
      REMAINING_ARGS=("$arg" "$@") # store remaining args BEFORE shifting so we still have the
      break
      ;;
    esac
    is_flag "$VAL" && fail_error "Flag '$FLAG' value '$VAL' looks like another flag"
    # run the command with the current value, which may be empty
    if ! $set_value_function "$FLAG" "$VAL"; then
      # the flag needs a value, so grab the next arg to use as the value
      VAL="$1" || fail_error "Could not get value for flag '$FLAG'"
      shift
      # fail if the next arg looks like a flag and not a value
      is_flag "$VAL" && fail_error "Flag '$FLAG' value '$VAL' looks like another flag"
      # fail because the flag needs a value and value given is empty, e.g., --namespace ''
      val_exists "$VAL" || fail_error "Flag '$FLAG' does not specify a value"
      # run the command again with the next arg as its value
      if ! $set_value_function "$FLAG" "$VAL"; then
        fail_error "Flag '$FLAG' must have a value" # probably won't reach this, but just in case
      fi
    fi
  done
}

# call this at the end of a command tree when there should be no more inputs past a given point.
# Usage: end_of_command_parsing "$@" # where "$@" contains the remaining args
function end_of_command_parsing() {
  if [[ "$#" -gt 0 ]]; then
    fail_error "Extraneous arguments at end of input: $*"
  fi
}

function info_msg() {
  echo -e "${INFO_PREFIX}" "$@"
}

function warn_msg() {
  echo -e "${WARNING_PREFIX}" "$@"
}

function error_msg() {
  echo -e "${ERROR_PREFIX}" "$@"
}

# run a kubectl command in the operator namespace
function KUBECTL_NS_OPERATOR() {
  $TOP_LEVEL_COMMAND --namespace "$ROOK_OPERATOR_NAMESPACE" "$@"
}

# run a kubectl command in the cluster namespace
function KUBECTL_NS_CLUSTER() {
  $TOP_LEVEL_COMMAND --namespace "$ROOK_CLUSTER_NAMESPACE" "$@"
}

####################################################################################################
# 'kubectl rook-ceph ceph ...' command
####################################################################################################

function run_ceph_command() {
  # do not call end_of_command_parsing here because all remaining input is passed directly to 'ceph'
  KUBECTL_NS_OPERATOR exec deploy/rook-ceph-operator -- ceph "$@" --connect-timeout=10 --conf="$CEPH_CONF_PATH"
}

####################################################################################################
# 'kubectl rook-ceph rbd ...' command
####################################################################################################

function run_rbd_command() {
  # do not call end_of_command_parsing here because all remaining input is passed directly to 'ceph'
  KUBECTL_NS_OPERATOR exec deploy/rook-ceph-operator -- rbd "$@" --conf="$CEPH_CONF_PATH"
}

####################################################################################################
# 'kubectl rook-ceph operator ...' commands
####################################################################################################

function run_operator_command() {
  if [ "$#" -eq 1 ] && [ "$1" = "restart" ]; then
    shift # remove the subcommand from the front of the arg list
    run_operator_restart_command "$@"
  elif [ "$#" -eq 3 ] && [ "$1" = "set" ]; then
    shift # remove the subcommand from the front of the arg list
    path_cm_rook_ceph_operator_config "$@"
  else
    fail_error "'operator' subcommand '$*' does not exist"
  fi
}

function run_operator_restart_command() {
  end_of_command_parsing "$@" # end of command tree
  KUBECTL_NS_OPERATOR rollout restart deploy/rook-ceph-operator
}

function path_cm_rook_ceph_operator_config() {
  if [[ "$#" -ne 2 ]]; then
    fail_error "require exactly 2 subcommand: $*"
  fi
  KUBECTL_NS_OPERATOR patch configmaps rook-ceph-operator-config --type json --patch "[{ op: replace, path: /data/$1, value: $2 }]"
}

####################################################################################################
# 'kubectl rook-ceph mons' commands
####################################################################################################

function run_mons_command() {
  if [ "$#" -ge 1 ] && [ "$1" = "restore-quorum" ]; then
    shift # remove the subcommand from the front of the arg list
    run_restore_quorum "$@"
  else
    fetch_mon_endpoints "$@"
  fi
}

function fetch_mon_endpoints() {
  end_of_command_parsing "$@" # end of command tree
  KUBECTL_NS_CLUSTER get cm rook-ceph-mon-endpoints -o json | jq --monochrome-output '.data.data' | tr -d '"' | tr -d '=' | sed 's/[A-Za-z]*//g'
}

function wait_for_deployment_to_be_running() {
  deployment=$1
  info_msg "Waiting for the pod from deployment \"$deployment\" to be running"
  KUBECTL_NS_CLUSTER wait deployment $deployment --for condition=Available=True --timeout=90s
}

function run_restore_quorum() {
  parse_flags parse_image_flag "$@" # parse flags before the good mon name
  [[ -z "${REMAINING_ARGS[0]:-""}" ]] && fail_error "Missing healthy mon name"
  good_mon="${REMAINING_ARGS[0]}"           # get the good mon being used to restore quorum
  shift                                     # remove the healthy mon from the front of the arg list
  REMAINING_ARGS=("${REMAINING_ARGS[@]:1}") # remove mon name from remaining args
  end_of_command_parsing "$@"               # end of command tree

  # Parse the endpoints configmap for the mon endpoints
  bad_mons=()
  mon_endpoints=$(KUBECTL_NS_CLUSTER get cm rook-ceph-mon-endpoints -o jsonpath='{.data.data}')
  # split the endpoints into an array, separated by the comma
  for single_mon in ${mon_endpoints//,/ }; do
    mon_name=$(echo "${single_mon/=/ }" | awk '{print $1}')
    mon_endpoint=$(echo "${single_mon/=/ }" | awk '{print $2}')
    echo "mon=$mon_name, endpoint=$mon_endpoint"
    if [ "$mon_name" = "$good_mon" ]; then
      good_mon_public_ip=$(echo "${mon_endpoint/:/ }" | awk '{print $1}')
      good_mon_port=$(echo "${mon_endpoint/:/ }" | awk '{print $2}')
    else
      bad_mons+=("$mon_name")
    fi
  done

  # Parse the cluster FSID
  ceph_fsid=$(KUBECTL_NS_CLUSTER get secret rook-ceph-mon -o jsonpath='{.data.fsid}' | base64 -d)
  if [ -z ${good_mon_public_ip+x} ]; then
    error_msg "error: good mon $good_mon not found"
    exit 1
  fi
  if [ "$ceph_fsid" = "" ]; then
    error_msg "ceph cluster fsid not found"
    exit 1
  fi

  # Check for the existence of the toolbox
  info_msg "Check for the running toolbox"
  wait_for_deployment_to_be_running rook-ceph-tools

  echo ""
  warn_msg "Restoring mon quorum to mon $good_mon ($good_mon_public_ip)"
  info_msg "The mons to discard are: ${bad_mons[*]}"
  info_msg "The cluster fsid is $ceph_fsid"
  prompt_to_continue_or_cancel "Are you sure you want to restore the quorum to mon \"$good_mon\"?" "yes-really-restore"

  # scale the operator deployment down
  KUBECTL_NS_OPERATOR scale deployment rook-ceph-operator --replicas=0

  # scale down all the mon pods
  KUBECTL_NS_CLUSTER scale deployment -l app=rook-ceph-mon --replicas=0

  # wait for the operator and mons to all stop
  info_msg "Waiting for operator and mon pods to stop"
  KUBECTL_NS_OPERATOR wait pod -l app=rook-ceph-operator --for=delete --timeout=90s
  KUBECTL_NS_CLUSTER wait pod -l app=rook-ceph-mon --for=delete --timeout=90s

  # start the mon debug pod
  run_start_debug rook-ceph-mon-$good_mon

  wait_for_deployment_to_be_running "rook-ceph-mon-$good_mon-debug"

  info_msg "Started debug pod, restoring the mon quorum in the debug pod"
  export monmap_path=/tmp/monmap

  # run some ceph commands in the mon debug pod to restore quorum
  set +eu
  info_msg "Extracting the monmap"
  KUBECTL_NS_CLUSTER exec deploy/rook-ceph-mon-$good_mon-debug -c mon -- ceph-mon \
    --fsid=$ceph_fsid \
    --keyring=/etc/ceph/keyring-store/keyring \
    --log-to-stderr=true \
    --err-to-stderr=true \
    --mon-cluster-log-to-stderr=true \
    --log-stderr-prefix=debug \
    --default-log-to-file=false \
    --default-mon-cluster-log-to-file=false \
    --mon-host=$ROOK_CEPH_MON_HOST \
    --mon-initial-members=$ROOK_CEPH_MON_INITIAL_MEMBERS \
    --id=$good_mon \
    --foreground \
    --public-addr=$good_mon_public_ip \
    --setuser-match-path=/var/lib/ceph/mon/ceph-$good_mon/store.db \
    --public-bind-addr=$ROOK_POD_IP \
    --extract-monmap=$monmap_path

  info_msg "Printing monmap"
  KUBECTL_NS_CLUSTER exec deploy/rook-ceph-mon-$good_mon-debug -c mon -- monmaptool --print $monmap_path

  # remove all the mons except the good one
  for bad_mon in "${bad_mons[@]}"; do
    info_msg "Removing mon $bad_mon"
    KUBECTL_NS_CLUSTER exec deploy/rook-ceph-mon-$good_mon-debug -c mon -- monmaptool $monmap_path --rm $bad_mon
  done

  info_msg "Injecting the monmap"
  KUBECTL_NS_CLUSTER exec deploy/rook-ceph-mon-$good_mon-debug -c mon -- ceph-mon \
    --fsid=$ceph_fsid \
    --keyring=/etc/ceph/keyring-store/keyring \
    --log-to-stderr=true \
    --err-to-stderr=true \
    --mon-cluster-log-to-stderr=true \
    --log-stderr-prefix=debug \
    --default-log-to-file=false \
    --default-mon-cluster-log-to-file=false \
    --mon-host=$ROOK_CEPH_MON_HOST \
    --mon-initial-members=$ROOK_CEPH_MON_INITIAL_MEMBERS \
    --id=$good_mon \
    --foreground \
    --public-addr=$good_mon_public_ip \
    --setuser-match-path=/var/lib/ceph/mon/ceph-$good_mon/store.db \
    --public-bind-addr=$ROOK_POD_IP \
    --inject-monmap=$monmap_path
  info_msg "Finished updating the monmap!"
  set -eu

  info_msg "Printing final monmap"
  KUBECTL_NS_CLUSTER exec deploy/rook-ceph-mon-$good_mon-debug -c mon -- monmaptool --print $monmap_path

  info_msg "Restoring the mons in the rook-ceph-mon-endpoints configmap to the good mon"
  KUBECTL_NS_CLUSTER patch configmaps rook-ceph-mon-endpoints --type json --patch "[{ op: replace, path: /data/data, value: $good_mon=$good_mon_public_ip:$good_mon_port }]"

  info_msg "Stopping the debug pod for mon $good_mon"
  run_stop_debug rook-ceph-mon-$good_mon

  info_msg "Check that the restored mon is responding"
  wait_for_mon_status_response

  info_msg "Purging the bad mons: ${bad_mons[*]}"
  # ignore errors purging old mons if their resources don't exist
  set +e
  for bad_mon in "${bad_mons[@]}"; do
    info_msg "purging old mon: $bad_mon"
    KUBECTL_NS_CLUSTER delete deploy rook-ceph-mon-$bad_mon
    KUBECTL_NS_CLUSTER delete svc rook-ceph-mon-$bad_mon
    info_msg "purging mon pvc if exists"
    KUBECTL_NS_CLUSTER delete pvc rook-ceph-mon-$bad_mon
  done
  set -e

  info_msg "Mon quorum was successfully restored to mon $good_mon"
  info_msg "Only a single mon is currently running"

  prompt_to_continue_or_cancel "Press Enter to start the operator and expand to full mon quorum again" ""

  # scale up the operator
  KUBECTL_NS_OPERATOR scale deployment rook-ceph-operator --replicas=1
  info_msg "The operator will now expand to full mon quorum"
}

function prompt_to_continue_or_cancel() {
  proceed_question=$1
  proceed_answer=$2
  if [[ "$proceed_answer" == "" ]]; then
    info_msg "$proceed_question"
  else
    info_msg "$proceed_question If so, enter: $proceed_answer"
  fi

  set +u
  if [[ "$ROOK_PLUGIN_SKIP_PROMPTS" != "true" ]]; then
    read INPUT_VAR
    if [[ "$proceed_answer" == "" ]]; then
      info_msg "continuing"
    elif [ "$INPUT_VAR" = "$proceed_answer" ]; then
      info_msg "proceeding"
    else
      warn_msg "cancelled"
      exit 1
    fi
  else
    warn_msg "skipped prompt since ROOK_PLUGIN_SKIP_PROMPTS=true"
  fi
  set -u
}

function wait_for_mon_status_response() {
  i=0
  max_retries=20
  sleep_time=5

  exit_status=1
  while [[ $exit_status != 0 ]]; do
    # Don't fail the script if the ceph command fails
    set +e
    KUBECTL_NS_CLUSTER exec deploy/rook-ceph-tools -- ceph status --connect-timeout=3
    exit_status=$?
    if [[ $exit_status = 0 ]]; then
      info_msg "finished waiting for ceph status"
    else
      info_msg "$i: waiting for ceph status to confirm single mon quorum"
      ((i++))
      if [[ $i -eq $max_retries ]]; then
        error_msg "timed out waiting for mon quorum to respond"
        exit 1
      fi
      info_msg "sleeping $sleep_time"
      sleep $sleep_time
    fi
    set -e
  done
}

####################################################################################################
# 'kubectl rook-ceph rook ...' commands
####################################################################################################

function rook_version() {
  [[ -z "${1:-""}" ]] && fail_error "Missing subcommand"
  subcommand="$1"
  shift # remove the subcommand from the front of the arg list
  case "$subcommand" in
  version)
    run_rook_version "$@"
    ;;
  status)
    run_rook_cr_status "$@"
    ;;
  purge-osd)
    run_purge_osd "$@"
    ;;
  *)
    fail_error "'rook' subcommand '$subcommand' does not exist"
    ;;
  esac
}

function run_rook_version() {
  end_of_command_parsing "$@" # end of command tree
  KUBECTL_NS_OPERATOR exec deploy/rook-ceph-operator -- rook version
}

function run_rook_cr_status() {
  if [ "$#" -eq 1 ] && [ "$1" = "all" ]; then
    cr_list=$(KUBECTL_NS_CLUSTER get crd | awk '{print $1}' | sed '1d')
    info_msg " CR status"
    for cr in $cr_list; do
      echo -e "$cr": "$(KUBECTL_NS_CLUSTER get "$cr" -ojson | jq --monochrome-output '.items[].status')"
    done
  elif [[ "$#" -eq 1 ]]; then
    KUBECTL_NS_CLUSTER get "$1" -ojson | jq --monochrome-output '.items[].status'
  elif [[ "$#" -eq 0 ]]; then
    KUBECTL_NS_CLUSTER get cephclusters.ceph.rook.io -ojson | jq --monochrome-output '.items[].status'
  else
    fail_error "$# does not exist"
  fi
}

function run_purge_osd() {
  force_removal=false
  if [ "$#" -eq 2 ] && [ "$2" = "--force" ]; then
    force_removal=true
  fi
  mon_endpoints=$(KUBECTL_NS_CLUSTER get cm rook-ceph-mon-endpoints -o jsonpath='{.data.data}' | cut -d "," -f1)
  ceph_secret=$(KUBECTL_NS_OPERATOR exec deploy/rook-ceph-operator -- cat /var/lib/rook/"$ROOK_CLUSTER_NAMESPACE"/client.admin.keyring | grep "key" | awk '{print $3}')
  KUBECTL_NS_OPERATOR exec deploy/rook-ceph-operator -- sh -c "export ROOK_MON_ENDPOINTS=$mon_endpoints \
      ROOK_CEPH_USERNAME=client.admin \
      ROOK_CEPH_SECRET=$ceph_secret \
      ROOK_CONFIG_DIR=/var/lib/rook && \
    rook ceph osd remove --osd-ids=$1 --force-osd-removal=$force_removal"
}

function run_cluster_health() {
  end_of_command_parsing "$@" # end of command tree
  set +e                      # if there are no pod that are not running, then do no exit
  check_mon_pods_nodes
  echo
  check_mon_quorum
  echo
  check_osd_pod_count_and_nodes
  echo
  check_all_pods_status
  echo
  check_pg_are_active_clean
  echo
  check_mgr_pods_status_and_count
  set -e
}

function check_mon_pods_nodes() {
  info_msg " Checking if at least three mon pods are running on different nodes"
  mon_unique_node_count=$(KUBECTL_NS_CLUSTER get pod -o wide | grep mon | grep -v canary | awk '{print $7}' | sort | uniq | wc -l)
  if [ "$mon_unique_node_count" -lt 3 ]; then
    warn_msg " At least three mon pods should running on different nodes"
  fi
  KUBECTL_NS_CLUSTER get pod | grep mon | grep --invert-match canary
}

function check_mon_quorum() {
  info_msg " Checking mon quorum and ceph health details"
  ceph_health_details=$(run_ceph_command health detail)
  if [[ "$ceph_health_details" = "HEALTH_OK" ]]; then
    echo -e "$ceph_health_details"
  elif [[ "$ceph_health_details" =~ "HEALTH_WARN" ]]; then
    warn_msg " $ceph_health_details"
  elif [[ "$ceph_health_details" =~ "HEALTH_ERR" ]]; then
    error_msg " $ceph_health_details"
  fi
}

function check_osd_pod_count_and_nodes() {
  info_msg " Checking if at least three osd pods are running on different nodes"
  osd_unique_node_count=$(KUBECTL_NS_CLUSTER get pod -o wide | grep osd | grep -v prepare | awk '{print $7}' | sort | uniq | wc -l)
  if [ "$osd_unique_node_count" -lt 3 ]; then
    warn_msg " At least three osd pods should running on different nodes"
  fi
  KUBECTL_NS_CLUSTER get pod | grep osd | grep --invert-match prepare
}

function check_all_pods_status() {
  info_msg " Pods that are in 'Running' status"
  KUBECTL_NS_OPERATOR get pod --field-selector status.phase=Running
  if [ "$ROOK_OPERATOR_NAMESPACE" != "$ROOK_CLUSTER_NAMESPACE" ]; then
    KUBECTL_NS_CLUSTER get pod --field-selector status.phase=Running
  fi

  echo
  warn_msg " Pods that are 'Not' in 'Running' status"
  KUBECTL_NS_OPERATOR get pod --field-selector status.phase!=Running | grep --invert-match Completed
  if [ "$ROOK_OPERATOR_NAMESPACE" != "$ROOK_CLUSTER_NAMESPACE" ]; then
    KUBECTL_NS_CLUSTER get pod --field-selector status.phase!=Running | grep --invert-match Completed
  fi
}

function check_pg_are_active_clean() {
  info_msg " checking placement group status"
  pg_state=$(run_ceph_command pg stat)
  pg_state_code=$(echo "${pg_state}" | awk '{print $4}')
  if [[ "$pg_state_code" = *"active+clean;"* ]]; then
    info_msg " $pg_state"
  elif [[ "$pg_state_code" =~ down || "$pg_state_code" =~ incomplete || "$pg_state_code" =~ snaptrim_error ]]; then
    error_msg " $pg_state"
  else
    warn_msg " $pg_state"
  fi
}

function check_mgr_pods_status_and_count() {
  info_msg " checking if at least one mgr pod is running"
  mgr_pod_count=$(KUBECTL_NS_CLUSTER get pod -o=custom-columns=NAME:.metadata.name,STATUS:.status.phase | grep mgr | awk '{print $2}' | sort | uniq | wc -l)
  if [ "$mgr_pod_count" -lt 1 ]; then
    error_msg " At least one mgr pod should running"
  fi
  KUBECTL_NS_CLUSTER get pod -o=custom-columns=NAME:.metadata.name,STATUS:.status.phase,NODE:.spec.nodeName | grep mgr
}

####################################################################################################
# 'kubectl rook-ceph debug commands
####################################################################################################

function remove_probe_from_deployment() {
  deployment_spec=$1
  deployment_spec=$(jq 'del(.template.spec.containers[0].livenessProbe)' <<<$deployment_spec)
  deployment_spec=$(jq 'del(.template.spec.containers[0].startupProbe)' <<<$deployment_spec)
  echo "$deployment_spec" # for returning the updated deployment_spec
}

function update_deployment_spec_command() {
  deployment_spec=$1
  main_command='["sleep","infinity"]'
  deployment_spec=$(jq .template.spec.containers[0].command="$main_command" <<<$deployment_spec)
  deployment_spec=$(jq .template.spec.containers[0].args=[] <<<$deployment_spec)
  echo "$deployment_spec" # for returning the updated deployment_spec
}

function update_deployment_spec_image() {
  deployment_spec=$1
  alternalte_image=\"$2\"
  deployment_spec=$(jq .template.spec.containers[0].image="$alternalte_image" <<<$deployment_spec)
  echo "$deployment_spec" # for returning the updated deployment_spec
}

function verify_debug_deployment() {
  if [[ $deployment_name != "rook-ceph-mon-"* ]] && [[ $deployment_name != "rook-ceph-osd-"* ]]; then
    fail_error "only mon or osd deployment name can passed for starting debug mode"
  fi
  if KUBECTL_NS_CLUSTER get deployment "$deployment_name" &>/dev/null; then
    echo "setting debug mode for \"$deployment_name\""
  else
    fail_error "deployment $deployment_name doesn't exist"
  fi
}

function run_start_debug() {
  # debug start can be used in different ways
  # 1) debug start deploymentName --alternate-image imageName
  # 2) debug start --alternate-image imageName deploymentName
  # 3) debug start deploymentName
  parse_flags parse_image_flag "$@" # parse flags before the deployment name
  [[ -z "${REMAINING_ARGS[0]:-""}" ]] && fail_error "Missing mon or osd deployment name"
  deployment_name="${REMAINING_ARGS[0]}"    # get deployment name
  REMAINING_ARGS=("${REMAINING_ARGS[@]:1}") # remove deploy name from remaining args
  set +u
  parse_flags parse_image_flag "${REMAINING_ARGS[@]}" # parse flags after the deployment name
  set -u

  verify_debug_deployment "$deployment_name"

  # copy the deployment spec before scaling it down
  deployment_spec=$(KUBECTL_NS_CLUSTER get deployment "$deployment_name" -o json | jq -r ".spec")
  # copy the deployment labels before scaling it down
  labels=$(KUBECTL_NS_CLUSTER get deployment "$deployment_name" -o json | jq -r ".metadata.labels")
  # add debug label to the list
  labels=$(echo "$labels" | jq '. + {"ceph.rook.io/do-not-reconcile": "true"}')
  # remove probes from the deployment
  deployment_spec=$(remove_probe_from_deployment "$deployment_spec")
  # update the deployment_spec with new image if alternate-image is passed
  if [ -n "$ALTERNATE_IMAGE" ]; then
    info_msg "setting debug image to \"$ALTERNATE_IMAGE\""
    deployment_spec=$(update_deployment_spec_image "$deployment_spec" "$ALTERNATE_IMAGE")
  fi
  # update the deployment_spec main container to be only a placeholder,
  info_msg "setting debug command to main container"
  deployment_spec=$(update_deployment_spec_command "$deployment_spec")

  # scale down the daemon pod if it's running
  set +e
  info_msg "get pod for deployment $deployment_name"
  deployment_pod=$(KUBECTL_NS_CLUSTER get pod | grep "$deployment_name" | awk '{ print $1  }')
  set -e
  if [ "$deployment_pod" != "" ]; then
    # scale the deployment to 0
    info_msg "scale down the deployment $deployment_name"
    KUBECTL_NS_CLUSTER scale deployments "$deployment_name" --replicas=0

    # wait for the deployment pod to be deleted
    info_msg "waiting for the deployment pod \"$deployment_pod\" to be deleted"
    KUBECTL_NS_CLUSTER wait --for=delete pod/"$deployment_pod" --timeout=60s
  fi

  # create debug deployment
  cat <<EOF | $TOP_LEVEL_COMMAND create -f -
    apiVersion: apps/v1
    kind: Deployment
    metadata:
        name: "$deployment_name-debug"
        namespace: "$ROOK_CLUSTER_NAMESPACE"
        labels:
            ${labels}
    spec:
        $deployment_spec
EOF
  info_msg "ensure the debug deployment $deployment_name is scaled up"
  KUBECTL_NS_CLUSTER scale deployments "$deployment_name-debug" --replicas=1
}

function run_stop_debug() {
  [[ -z "${1:-""}" ]] && fail_error "Missing mon or osd deployment name"
  deployment_name="$1"
  if [[ $deployment_name != *"debug" ]]; then
    deployment_name="$deployment_name-debug"
  fi
  verify_debug_deployment "$deployment_name"
  info_msg "removing debug mode from \"$deployment_name\""

  # delete the deployment debug pod
  KUBECTL_NS_CLUSTER delete deployment "$deployment_name"
  # remove the debug suffix from the deployment
  deployment_name="${deployment_name%'-debug'}"
  # scale the deployment to 1
  KUBECTL_NS_CLUSTER scale deployments "$deployment_name" --replicas=1
}

function parse_image_flag() {
  local flag="$1"
  local val="$2"
  case "$flag" in
  "--alternate-image")
    val_exists "$val" || return 1 # val should exist
    ALTERNATE_IMAGE="${val}"
    ;;
  *)
    fail_error "Flag $flag is not supported"
    ;;
  esac
}

function run_debug() {
  [[ -z "${1:-""}" ]] && fail_error "Missing 'debug' subcommand"
  subcommand="$1"
  shift # remove the subcommand from the front of the arg list
  case "$subcommand" in
  start)
    run_start_debug "$@"
    ;;
  stop)
    run_stop_debug "$@"
    ;;
  *)
    fail_error "'debug' subcommand '$subcommand' does not exist"
    ;;
  esac
}

function run_dr_subcommands() {
  [[ -z "${1:-""}" ]] && fail_error "Missing subcommand"
  subcommand="$1"
  shift # remove the subcommand from the front of the arg list
  case $subcommand in
  health)
    run_dr_health "$@"
    ;;
  *)
    fail_error "'dr' subcommand '$subcommand' does not exist"
    ;;
  esac
}

function run_dr_health() {
  # do not call end_of_command_parsing here because all remaining input is passed directly to 'ceph'
  info_msg "fetching the cephblockpools with mirroring enabled"
  blockpool_List=$(KUBECTL_NS_CLUSTER get cephblockpool | awk '{print $1}' | sed "1 d")
  blockpool_name=""
  for blockpool in $blockpool_List; do
    mirroring_enabled=$(KUBECTL_NS_CLUSTER get cephblockpool "$blockpool" -o jsonpath='{.spec.mirroring.enabled}')
    if [ "$mirroring_enabled" == "true" ]; then
      blockpool_name+=$blockpool
      info_msg "found '$blockpool' cephblockpool with mirroring enabled"
      break
    fi
  done

  [ -z "$blockpool_name" ] && error_msg "DR is not confiqured, cephblockpool with mirroring enabled not found."

  # `sed -e 's/^.//' -e 's/.$//'` is used to remove first and last char. It is used twice below to remove `["` form start and `"]` from end.
  SECRET_NAME=$(KUBECTL_NS_CLUSTER get cephblockpool "$blockpool_name" -ojsonpath='{.spec.mirroring.peers.secretNames}' | sed -e 's/^.//' -e 's/.$//' | sed -e 's/^.//' -e 's/.$//')
  MON_HOST=$(KUBECTL_NS_CLUSTER get secret/"$SECRET_NAME" -o jsonpath='{.data.token}' | base64 --decode | base64 --decode | jq '.mon_host' | sed -e 's/^.//' -e 's/.$//' | sed -e 's/^.//' -e 's/.$//')
  PEER_KEY=$(KUBECTL_NS_CLUSTER get secret/"$SECRET_NAME" -o jsonpath='{.data.token}' | base64 --decode | base64 --decode | jq '.key' | sed -e 's/^.//' -e 's/.$//')
  # run ceph status command form one cluster to another
  echo
  info_msg "running ceph status from peer cluster"
  set +e
  run_ceph_command -s --mon-host "$MON_HOST" --id rbd-mirror-peer --key "$PEER_KEY" "$@"
  exitCode=$?
  if [ $exitCode -ne 0 ]; then
    error_msg "failed to get ceph status from peer cluster, please check for network issues between the clusters"
  fi
  # run rbd pool mirror status
  info_msg "running mirroring daemon health"
  run_rbd_command -p "$blockpool_name" mirror pool status
  set -e
}

function debug_csi() {
  [[ -z "${1:-""}" ]] && fail_error "Missing csi-debug subcommand"
  subcommand="$1"
  shift # remove the subcommand from the front of the arg list

  case $subcommand in
  dmesg)
    run_dmesg "$@"
    ;;
  *)
    fail_error "'csi-debug' subcommand '$subcommand' does not exit"
    ;;
  esac
}

function get_csi_node_and_driver_name() {
  local nodeName driverName pvcName
  nodeName=$(KUBECTL_NS_CLUSTER get pods "$1" -ojson | jq '.spec.nodeName' | tr -d '"')
  pvcName=$(KUBECTL_NS_CLUSTER get pod "$1" -ojson | jq '.spec.volumes[0].persistentVolumeClaim.claimName' | tr -d '"')
  volumeProvisoiner=$(KUBECTL_NS_CLUSTER get pvc rbd-pvc -ojson | jq '.metadata.annotations["volume.kubernetes.io/storage-provisioner"]')

  if [[ "$volumeProvisoiner" =~ "rbd" ]]; then
    driverName="csi-rbdplugin"
  else
    driverName="csi-cephfsplugin"
  fi

  echo "$nodeName" "$driverName" "$pvcName"
}

function run_dmesg() {
  [[ "$#" -gt 1 ]] && fail_error "Extraneous arguments at end of input: $*"
  [[ -z "${1:-""}" ]] && fail_error "Missing 'csi-debug dmesg' arg node name"
  podName="$1"

  node_driver_pvc_name="$(get_csi_node_and_driver_name "$podName")"
  IFS=" " read -r nodeName driverName pvcName <<<"$node_driver_pvc_name"

  echo $nodeName $driverName
  csi_rbdplugin_pod=$(KUBECTL_NS_OPERATOR get pods --no-headers -o custom-columns=":metadata.name" --field-selector spec.nodeName="$nodeName" | grep "$driverName" | grep -v "provisioner")
  info_msg "running dmesg command on pod $csi_rbdplugin_pod on node $nodeName"
  echo
  KUBECTL_NS_OPERATOR exec "$csi_rbdplugin_pod" -c "$driverName" -- dmesg
}

####################################################################################################
# 'kubectl rook-ceph status' command
####################################################################################################
# Disabling it for now, will enable once it is ready implementation

# The status subcommand takes some args
# LONG_STATUS='false'

# # set_value_function for parsing flags for the status subcommand.
# function parse_status_flag () {
#   local flag="$1"
#   local val="$2"
#   case "$flag" in
#     "-l"|"--long")
#       flag_no_value "$flag" "$val"
#       LONG_STATUS='true'
#       ;;
#     *)
#       fail_error "Unsupported 'status' flag '$flag'"
#       ;;
#   esac
# }

# function run_status_command () {
#   REMAINING_ARGS=()
#   parse_flags 'parse_status_flag' "$@"
#   end_of_command_parsing "${REMAINING_ARGS[@]}" # end of command tree

#   if [[ "$LONG_STATUS" == "true" ]]; then
#     echo "LONG STATUS"
#   else
#     echo "SHORT STATUS"
#   fi
# }

####################################################################################################
# MAIN COMMAND HANDLER (is effectively main)
####################################################################################################

function run_main_command() {
  local command="$1"
  shift # pop first arg off the front of the function arg list
  case "$command" in
  ceph)
    run_ceph_command "$@"
    ;;
  rbd)
    run_rbd_command "$@"
    ;;
  operator)
    run_operator_command "$@"
    ;;
  mons)
    run_mons_command "$@"
    ;;
  rook)
    rook_version "$@"
    ;;
  health)
    run_cluster_health "$@"
    ;;
  debug)
    run_debug "$@"
    ;;
  dr)
    run_dr_subcommands "$@"
    ;;
  csi-debug)
    debug_csi "$@"
    ;;
  # status)
  #   run_status_command "$@"
  #   ;;
  *)
    fail_error "Unknown command '$command'"
    ;;
  esac
}

####################################################################################################
# MAIN: PARSE MAIN ARGS AND CALL MAIN COMMAND HANDLER
####################################################################################################

# set_value_function for parsing flags for the main rook-ceph plugin.
function parse_main_flag() {
  local flag="$1"
  local val="$2"
  case "$flag" in
  "-n" | "--namespace")
    val_exists "$val" || return 1 # val should exist
    ROOK_CLUSTER_NAMESPACE="${val}"
    ;;
  "-h" | "--help")
    flag_no_value "$flag" "$val"
    print_usage
    exit 0 # unique for the help flag; stop parsing everything and exit with success
    ;;
  "-o" | "--operator-namespace")
    val_exists "$val" || return 1 # val should exist
    ROOK_OPERATOR_NAMESPACE="${val}"
    ;;
  "--context")
    val_exists "$val" || return 1 # val should exist
    TOP_LEVEL_COMMAND="kubectl --context=${val}"
    ;;
  *)
    fail_error "Flag $flag is not supported"
    ;;
  esac
}

REMAINING_ARGS=()
parse_flags 'parse_main_flag' "$@"

# Note: don't move default values
# Default values
: "${ROOK_CLUSTER_NAMESPACE:=rook-ceph}"
: "${ROOK_OPERATOR_NAMESPACE:=$ROOK_CLUSTER_NAMESPACE}"
: "${TOP_LEVEL_COMMAND:=kubectl}"
: "${ALTERNATE_IMAGE=}"
: "${RESET:=\033[0m}"                           # For no color
: "${ERROR_PREFIX:=\033[1;31mError:$RESET}"     # \033[1;31m for Red color
: "${INFO_PREFIX:=\033[1;34mInfo:$RESET}"       # \033[1;34m for Blue color
: "${WARNING_PREFIX:=\033[1;33mWarning:$RESET}" # \033[1;33m for Yellow color

if [[ "${#REMAINING_ARGS[@]}" -eq 0 ]]; then
  fail_error "No command to run"
fi

# Default value
CEPH_CONF_PATH="/var/lib/rook/$ROOK_CLUSTER_NAMESPACE/$ROOK_CLUSTER_NAMESPACE.config" # path of ceph config

run_main_command "${REMAINING_ARGS[@]}"
