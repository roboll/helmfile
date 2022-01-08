FROM alpine:3.13

RUN apk add --no-cache ca-certificates git bash curl jq

ARG TARGETARCH
ARG HELM_VERSION="v3.7.2"
ARG HELM_LOCATION="https://get.helm.sh"
ARG HELM_FILENAME="helm-${HELM_VERSION}-linux-${TARGETARCH}.tar.gz"

RUN set -x && \
    wget ${HELM_LOCATION}/${HELM_FILENAME}.sha256sum && \
    HELM_SHA256=$(cat ${HELM_FILENAME}.sha256sum) && \
    wget ${HELM_LOCATION}/${HELM_FILENAME} && \
    echo Verifying ${HELM_FILENAME}... && \
    sha256sum ${HELM_FILENAME} | grep "${HELM_SHA256}" && \
    echo Extracting ${HELM_FILENAME}... && \
    tar zxvf ${HELM_FILENAME} && mv /linux-${TARGETARCH}/helm /usr/local/bin/ && \
    rm ${HELM_FILENAME} && rm -r /linux-${TARGETARCH}

# using the install documentation found at https://kubernetes.io/docs/tasks/tools/install-kubectl/
# for now but in a future version of alpine (in the testing version at the time of writing)
# we should be able to install using apk add.
# the sha256 sum can be found at https://storage.googleapis.com/kubernetes-release/release/${KUBECTL_VERSION}/bin/linux/${TARGETARCH}/kubectl.sha256
# maybe a good idea to automate in the future?
ENV KUBECTL_VERSION="v1.21.4"
RUN set -x && \
    curl --retry 5 --retry-connrefused -LO "https://storage.googleapis.com/kubernetes-release/release/${KUBECTL_VERSION}/bin/linux/${TARGETARCH}/kubectl.sha256" && \
    KUBECTL_SHA256=$(cat kubectl.sha256) && \
    curl --retry 5 --retry-connrefused -LO "https://storage.googleapis.com/kubernetes-release/release/${KUBECTL_VERSION}/bin/linux/${TARGETARCH}/kubectl" && \
    sha256sum kubectl | grep ${KUBECTL_SHA256} && \
    chmod +x kubectl && \
    mv kubectl /usr/local/bin/kubectl

ENV KUSTOMIZE_VERSION="v3.8.8"
RUN set -x && \
    curl --retry 5 --retry-connrefused -LO https://github.com/kubernetes-sigs/kustomize/releases/download/kustomize/${KUSTOMIZE_VERSION}/checksums.txt && \
    KUSTOMIZE_SHA256=$(grep kustomize_${KUSTOMIZE_VERSION}_linux_${TARGETARCH}.tar.gz checksums.txt | awk '{print $1}') && \
    curl --retry 5 --retry-connrefused -LO https://github.com/kubernetes-sigs/kustomize/releases/download/kustomize/${KUSTOMIZE_VERSION}/kustomize_${KUSTOMIZE_VERSION}_linux_${TARGETARCH}.tar.gz && \
    sha256sum kustomize_${KUSTOMIZE_VERSION}_linux_${TARGETARCH}.tar.gz | grep ${KUSTOMIZE_SHA256} && \
    tar zxf kustomize_${KUSTOMIZE_VERSION}_linux_${TARGETARCH}.tar.gz && \
    rm kustomize_${KUSTOMIZE_VERSION}_linux_${TARGETARCH}.tar.gz && \
    mv kustomize /usr/local/bin/kustomize

RUN set -x && \
    helm plugin install https://github.com/databus23/helm-diff --version v3.2.0 && \
    helm plugin install https://github.com/jkroepke/helm-secrets --version v3.5.0 && \
    helm plugin install https://github.com/hypnoglow/helm-s3.git --version v0.10.0 && \
    helm plugin install https://github.com/aslafy-z/helm-git.git --version v0.10.0

COPY dist/helmfile_linux_${TARGETARCH} /usr/local/bin/helmfile

CMD ["/usr/local/bin/helmfile"]
