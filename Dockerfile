FROM golang:1.10

WORKDIR /go/src/app
COPY . .

RUN go get -d -v ./...
RUN go build -v -o helmfile

FROM debian:stretch

RUN apt-get update && apt-get install -y wget curl git lsb-release sudo
ENV HELM_VERSION=v2.10.0
ENV HELM_LOCATION="https://kubernetes-helm.storage.googleapis.com"
ENV HELM_FILENAME="helm-${HELM_VERSION}-linux-amd64.tar.gz"
RUN wget ${HELM_LOCATION}/${HELM_FILENAME} && \
    tar zxf ${HELM_FILENAME} && mv /linux-amd64/helm /usr/local/bin/ && \
    rm ${HELM_FILENAME} && rm -r /linux-amd64

COPY --from=0 /go/src/app/helmfile /usr/local/bin/helmfile

RUN mkdir -p $(helm home)/plugins
RUN helm plugin install https://github.com/databus23/helm-diff --version 2.10.0+1
#ENV TARBALL_URL https://github.com/databus23/helm-diff/releases/download/v2.10.0%2B1/helm-diff-linux.tgz
#RUN wget -O- $TARBALL_URL | tar -C $(helm home)/plugins -xzv

#### helm-secrets not working with Alpine
####work around distro detection
RUN helm plugin install https://github.com/futuresimple/helm-secrets

ADD https://storage.googleapis.com/kubernetes-release/release/v1.10.0/bin/linux/amd64/kubectl /usr/local/bin/kubectl
RUN chmod +x /usr/local/bin/kubectl


#ENTRYPOINT ["/usr/local/bin/helmfile"]
CMD ["helmfile"]
