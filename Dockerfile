FROM golang:1.10.1-alpine3.7 as builder

RUN apk add --no-cache make git
WORKDIR /go/src/github.com/roboll/helmfile/
COPY . /go/src/github.com/roboll/helmfile/
RUN make static-linux

# -----------------------------------------------------------------------------

FROM alpine:3.7

RUN apk add --no-cache ca-certificates git bash

ARG HELM_VERSION=v2.9.1
ARG HELM_LOCATION="https://kubernetes-helm.storage.googleapis.com"
ARG HELM_FILENAME="helm-${HELM_VERSION}-linux-amd64.tar.gz"
ARG HELM_SHA256="56ae2d5d08c68d6e7400d462d6ed10c929effac929fedce18d2636a9b4e166ba"
RUN wget ${HELM_LOCATION}/${HELM_FILENAME} && \
    sha256sum ${HELM_FILENAME} | grep -q "${HELM_SHA256}" && \
    tar zxf ${HELM_FILENAME} && mv /linux-amd64/helm /usr/local/bin/ && \
    rm ${HELM_FILENAME} && rm -r /linux-amd64

RUN mkdir -p "$(helm home)/plugins"
RUN helm plugin install https://github.com/databus23/helm-diff

COPY --from=builder /go/src/github.com/roboll/helmfile/dist/helmfile_linux_amd64 /usr/local/bin/helmfile

CMD ["/usr/local/bin/helmfile"]
