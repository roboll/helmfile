FROM golang:1.11.2-alpine3.8 as builder

RUN apk add --no-cache make git
WORKDIR /go/src/github.com/roboll/helmfile/
COPY . /go/src/github.com/roboll/helmfile/
RUN make static-linux

# -----------------------------------------------------------------------------

FROM alpine:3.8

RUN apk add --no-cache ca-certificates git bash curl

ARG HELM_VERSION=v2.12.0
ARG HELM_LOCATION="https://kubernetes-helm.storage.googleapis.com"
ARG HELM_FILENAME="helm-${HELM_VERSION}-linux-amd64.tar.gz"
ARG HELM_SHA256="9f96a6e4fc52b5df906da381532cc2eb2f3f57cc203ccaec2b11cf5dc26a7dfc"
RUN wget ${HELM_LOCATION}/${HELM_FILENAME} && \
    sha256sum ${HELM_FILENAME} | grep -q "${HELM_SHA256}" && \
    tar zxf ${HELM_FILENAME} && mv /linux-amd64/helm /usr/local/bin/ && \
    rm ${HELM_FILENAME} && rm -r /linux-amd64

RUN mkdir -p "$(helm home)/plugins"
RUN helm plugin install https://github.com/databus23/helm-diff && \
    helm plugin install https://github.com/futuresimple/helm-secrets

COPY --from=builder /go/src/github.com/roboll/helmfile/dist/helmfile_linux_amd64 /usr/local/bin/helmfile

CMD ["/usr/local/bin/helmfile"]
