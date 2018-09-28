#!/usr/bin/env bash

# IMPORTS -----------------------------------------------------------------------------------------------------------

# determine working directory to use to relative paths irrespective of starting directory
dir="${BASH_SOURCE%/*}"
if [[ ! -d "${dir}" ]]; then dir="${PWD}"; fi

. "${dir}/lib/output.sh"
. "${dir}/lib/ensure.sh"


# GLOBALS -----------------------------------------------------------------------------------------------------------

test_ns="helmfile-tests-$(date +"%Y%m%d-%H%M%S")"
helmfile="./helmfile --namespace=${test_ns}"
helm="helm --kube-context=minikube"
kubectl="kubectl --context=minikube --namespace=${test_ns}"

# FUNCTIONS ----------------------------------------------------------------------------------------------------------

function wait_deploy_ready() {
    ${kubectl} rollout status deployment ${1}
    while [ "$(${kubectl} get deploy ${1} -o=jsonpath='{.status.readyReplicas}')" == "0" ]; do
        info "Waiting for deployment ${1} to be ready"
        sleep 1
    done
}
function retry() {
    local -r max=${1}
    local -r command=${2}
    n=0
    retry_result=0
    until [ ${n} -ge ${max} ]; do
        info "Executing: ${command} (attempt $((n+1)))"
        ${command} && break  # substitute your command here
        retry_result=$?
        n=$[$n+1]
        # approximated binary exponential backoff to reduce flakiness
        sleep $((n ** 2))
    done
}

# SETUP --------------------------------------------------------------------------------------------------------------

set -e
info "Using namespace: ${test_ns}"
info "Using Helm version: $(helm version --short --client | grep -o v.*$)"
${helm} init --wait --override spec.template.spec.automountServiceAccountToken=true
${kubectl} get namespace ${test_ns} &> /dev/null && warn "Namespace ${test_ns} exists, from a previous test run?"
$kubectl create namespace ${test_ns} || fail "Could not create namespace ${test_ns}"
trap "{ $kubectl delete namespace ${test_ns}; }" EXIT # remove namespace whenever we exit this script


# TEST CASES----------------------------------------------------------------------------------------------------------

test_start "happypath - simple rollout of httpbin chart"
info "Syncing ${dir}/happypath.yaml"
${helmfile} -f ${dir}/happypath.yaml sync
wait_deploy_ready httpbin-httpbin
retry 5 "curl --fail $(minikube service --url --namespace=${test_ns} httpbin-httpbin)/status/200"
[ ${retry_result} -eq 0 ] || fail "httpbin failed to return 200 OK"
info "Deleting release"
${helmfile} -f ${dir}/happypath.yaml delete
${helm} status --namespace=${test_ns} httpbin &> /dev/null && fail "release should not exist anymore after a delete"
test_pass "happypath"


# ALL DONE -----------------------------------------------------------------------------------------------------------

all_tests_passed
