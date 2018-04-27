FROM golang:1.10.1-alpine3.7 as builder

RUN apk add --no-cache make git
WORKDIR /go/src/github.com/roboll/helmfile/
COPY . /go/src/github.com/roboll/helmfile/
RUN make static-linux

# -----------------------------------------------------------------------------

FROM alpine:3.7

RUN apk add --no-cache ca-certificates

ENV HELM_VERSION=v2.8.2
ENV HELM_LOCATION="https://kubernetes-helm.storage.googleapis.com"
ENV HELM_FILENAME="helm-${HELM_VERSION}-linux-amd64.tar.gz"
ENV HELM_SHA256="614b5ac79de4336b37c9b26d528c6f2b94ee6ccacb94b0f4b8d9583a8dd122d3"
RUN wget ${HELM_LOCATION}/${HELM_FILENAME} && \
    sha256sum ${HELM_FILENAME} | grep -q "${HELM_SHA256}" && \
    tar zxf ${HELM_FILENAME} && mv /linux-amd64/helm /usr/local/bin/ && \
    rm ${HELM_FILENAME} && rm -r /linux-amd64

COPY --from=builder /go/src/github.com/roboll/helmfile/dist/helmfile_linux_amd64 /usr/local/bin/helmfile

CMD ["/usr/local/bin/helmfile"]
