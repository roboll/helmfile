FROM golang:1.12.4-alpine3.9 as builder

RUN apk add --no-cache make git
WORKDIR /workspace/helmfile
COPY . /workspace/helmfile
RUN make static-linux

# -----------------------------------------------------------------------------

FROM alpine:3.8

RUN apk add --no-cache ca-certificates git bash curl jq

ARG HELM_VERSION=v2.13.0
ARG HELM_LOCATION="https://kubernetes-helm.storage.googleapis.com"
ARG HELM_FILENAME="helm-${HELM_VERSION}-linux-amd64.tar.gz"
ARG HELM_SHA256="15eca6ad225a8279de80c7ced42305e24bc5ac60bb7d96f2d2fa4af86e02c794"
RUN wget ${HELM_LOCATION}/${HELM_FILENAME} && \
    sha256sum ${HELM_FILENAME} | grep -q "${HELM_SHA256}" && \
    tar zxf ${HELM_FILENAME} && mv /linux-amd64/helm /usr/local/bin/ && \
    rm ${HELM_FILENAME} && rm -r /linux-amd64

# using the install documentation found at https://kubernetes.io/docs/tasks/tools/install-kubectl/
# for now but in a future version of alpine (in the testing version at the time of writing)
# we should be able to install using apk add.
ENV KUBECTL_VERSION="v1.14.5"
ENV KUBECTL_SHA256="26681319de56820a8467c9407e9203d5b15fb010ffc75ac5b99c9945ad0bd28c"
RUN curl -LO "https://storage.googleapis.com/kubernetes-release/release/${KUBECTL_VERSION}/bin/linux/amd64/kubectl" && \
    sha256sum kubectl | grep ${KUBECTL_SHA256} && \
    chmod +x kubectl && \
    mv kubectl /usr/local/bin/kubectl

RUN mkdir -p "$(helm home)/plugins"
RUN helm plugin install https://github.com/databus23/helm-diff && \
    helm plugin install https://github.com/futuresimple/helm-secrets && \
    helm plugin install https://github.com/hypnoglow/helm-s3.git && \
    helm plugin install https://github.com/aslafy-z/helm-git.git && \
    helm plugin install https://github.com/rimusz/helm-tiller

COPY --from=builder /workspace/helmfile/dist/helmfile_linux_amd64 /usr/local/bin/helmfile

CMD ["/usr/local/bin/helmfile"]
