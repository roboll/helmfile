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
# helm v2
if helm version --client 2>/dev/null | grep '"v2\.'; then
  info "Using Helm version: $(helm version --short --client | grep -o v.*$)"
  ${helm} init --wait --override spec.template.spec.automountServiceAccountToken=true
# helm v3
else
  info "Using Helm version: $(helm version --short | grep -o v.*$)"
fi
${helm} plugin install https://github.com/databus23/helm-diff --version v3.0.0-rc.7
${kubectl} get namespace ${test_ns} &> /dev/null && warn "Namespace ${test_ns} exists, from a previous test run?"
$kubectl create namespace ${test_ns} || fail "Could not create namespace ${test_ns}"
trap "{ $kubectl delete namespace ${test_ns}; }" EXIT # remove namespace whenever we exit this script


# TEST CASES----------------------------------------------------------------------------------------------------------

test_start "happypath - simple rollout of httpbin chart"

info "Diffing ${dir}/happypath.yaml"
bash -c "${helmfile} -f ${dir}/happypath.yaml diff --detailed-exitcode; code="'$?'"; [ "'${code}'" -eq 2 ]" || fail "unexpected exit code returned by helmfile diff"

info "Diffing ${dir}/happypath.yaml without color"
bash -c "${helmfile} -f ${dir}/happypath.yaml --no-color diff --detailed-exitcode; code="'$?'"; [ "'${code}'" -eq 2 ]" || fail "unexpected exit code returned by helmfile diff"

info "Diffing ${dir}/happypath.yaml with limited context"
bash -c "${helmfile} -f ${dir}/happypath.yaml diff --context 3 --detailed-exitcode; code="'$?'"; [ "'${code}'" -eq 2 ]" || fail "unexpected exit code returned by helmfile diff"

info "Templating ${dir}/happypath.yaml"
${helmfile} -f ${dir}/happypath.yaml template
code=$?
[ ${code} -eq 0 ] || fail "unexpected exit code returned by helmfile template: ${code}"

info "Applying ${dir}/happypath.yaml"
bash -c "${helmfile} -f ${dir}/happypath.yaml apply --detailed-exitcode; code="'$?'"; echo Code: "'$code'"; [ "'${code}'" -eq 2 ]" || fail "unexpected exit code returned by helmfile apply"

info "Syncing ${dir}/happypath.yaml"
${helmfile} -f ${dir}/happypath.yaml sync
wait_deploy_ready httpbin-httpbin
retry 5 "curl --fail $(minikube service --url --namespace=${test_ns} httpbin-httpbin)/status/200"
[ ${retry_result} -eq 0 ] || fail "httpbin failed to return 200 OK"

info "Applying ${dir}/happypath.yaml"
${helmfile} -f ${dir}/happypath.yaml apply --detailed-exitcode
code=$?
[ ${code} -eq 0 ] || fail "unexpected exit code returned by helmfile apply: want 0, got ${code}"

info "Locking dependencies"
${helmfile} -f ${dir}/happypath.yaml deps
code=$?
[ ${code} -eq 0 ] || fail "unexpected exit code returned by helmfile deps: ${code}"

info "Applying ${dir}/happypath.yaml with locked dependencies"
${helmfile} -f ${dir}/happypath.yaml apply
code=$?
[ ${code} -eq 0 ] || fail "unexpected exit code returned by helmfile apply: ${code}"
${helm} list --namespace=${test_ns} || fail "unable to list releases"

info "Deleting release"
${helmfile} -f ${dir}/happypath.yaml delete
${helm} status --namespace=${test_ns} httpbin &> /dev/null && fail "release should not exist anymore after a delete"

info "Ensuring \"helmfile delete\" doesn't fail when no releases installed"
${helmfile} -f ${dir}/happypath.yaml delete || fail "\"helmfile delete\" shouldn't fail when there are no installed releases"

info "Ensuring \"helmfile template\" output does contain only YAML docs"
(${helmfile} -f ${dir}/happypath.yaml template | kubectl apply -f -) || fail "\"helmfile template | kubectl apply -f -\" shouldn't fail"

test_pass "happypath"


# ALL DONE -----------------------------------------------------------------------------------------------------------

all_tests_passed
