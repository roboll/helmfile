FROM alpine:3.4

RUN apk add --update curl && \
	curl -L https://kubernetes-helm.storage.googleapis.com/helm-v2.0.0-linux-amd64.tar.gz | \
	tar zxf - && mv /linux-amd64/helm /usr/local/bin/ && rm -r /linux-amd64 && apk del curl

COPY dist/helmfile_linux_amd64 /usr/local/bin/helmfile

CMD ["/usr/local/bin/helmfile"]
