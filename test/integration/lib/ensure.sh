#!/usr/bin/env bash

# Check environment is correctly setup

if ! kubectl version --short 1> /dev/null; then
    fail "Could not connect to minikube apiserver"
fi
if ! hash curl 1>/dev/null; then
    fail "curl needs to be installed."
fi
if ! hash docker 1>/dev/null; then
    fail "Docker needs to be installed."
fi
if ! docker version 1> /dev/null; then
    fail "Could not connect to Docker daemon"
fi
if ! hash helm 1>/dev/null; then
    fail "Helm needs to be installed."
fi
if [ -z "${KUBECONTEXT}" ]; then
  fail "KUBECONTEXT needs to be set."
fi
