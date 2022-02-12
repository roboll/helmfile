FROM golang:1.17.3-alpine3.13 as builder

RUN apk add --no-cache make git
WORKDIR /workspace/helmfile
COPY . /workspace/helmfile
RUN make static-linux

# -----------------------------------------------------------------------------

FROM alpine:3.13

RUN apk add --no-cache ca-certificates git bash curl jq

ARG HELM_VERSION="v3.7.2"
ARG HELM_SHA256="4ae30e48966aba5f807a4e140dad6736ee1a392940101e4d79ffb4ee86200a9e"
ARG HELM_LOCATION="https://get.helm.sh"
ARG HELM_FILENAME="helm-${HELM_VERSION}-linux-amd64.tar.gz"

RUN set -x && \
    wget ${HELM_LOCATION}/${HELM_FILENAME} && \
    echo Verifying ${HELM_FILENAME}... && \
    sha256sum ${HELM_FILENAME} | grep -q "${HELM_SHA256}" && \
    echo Extracting ${HELM_FILENAME}... && \
    tar zxvf ${HELM_FILENAME} && mv /linux-amd64/helm /usr/local/bin/ && \
    rm ${HELM_FILENAME} && rm -r /linux-amd64

# using the install documentation found at https://kubernetes.io/docs/tasks/tools/install-kubectl/
# for now but in a future version of alpine (in the testing version at the time of writing)
# we should be able to install using apk add.
# the sha256 sum can be found at https://storage.googleapis.com/kubernetes-release/release/${KUBECTL_VERSION}/bin/linux/amd64/kubectl.sha256
# maybe a good idea to automate in the future?
ENV KUBECTL_VERSION="v1.21.4"
ENV KUBECTL_SHA256="9410572396fb31e49d088f9816beaebad7420c7686697578691be1651d3bf85a"
RUN set -x && \
    curl --retry 5 --retry-connrefused -LO "https://storage.googleapis.com/kubernetes-release/release/${KUBECTL_VERSION}/bin/linux/amd64/kubectl" && \
    sha256sum kubectl | grep ${KUBECTL_SHA256} && \
    chmod +x kubectl && \
    mv kubectl /usr/local/bin/kubectl

ENV KUSTOMIZE_VERSION="v3.8.8"
ENV KUSTOMIZE_SHA256="175938206f23956ec18dac3da0816ea5b5b485a8493a839da278faac82e3c303"
RUN set -x && \
    curl --retry 5 --retry-connrefused -LO https://github.com/kubernetes-sigs/kustomize/releases/download/kustomize/${KUSTOMIZE_VERSION}/kustomize_${KUSTOMIZE_VERSION}_linux_amd64.tar.gz && \
    sha256sum kustomize_${KUSTOMIZE_VERSION}_linux_amd64.tar.gz | grep ${KUSTOMIZE_SHA256} && \
    tar zxf kustomize_${KUSTOMIZE_VERSION}_linux_amd64.tar.gz && \
    rm kustomize_${KUSTOMIZE_VERSION}_linux_amd64.tar.gz && \
    mv kustomize /usr/local/bin/kustomize

RUN helm plugin install https://github.com/databus23/helm-diff --version v3.3.1 && \
    helm plugin install https://github.com/jkroepke/helm-secrets --version v3.5.0 && \
    helm plugin install https://github.com/hypnoglow/helm-s3.git --version v0.10.0 && \
    helm plugin install https://github.com/aslafy-z/helm-git.git --version v0.10.0

# Allow users other than root to use helm plugins located in root home
RUN chmod 751 /root

COPY --from=builder /workspace/helmfile/dist/helmfile_linux_amd64 /usr/local/bin/helmfile

CMD ["/usr/local/bin/helmfile"]
