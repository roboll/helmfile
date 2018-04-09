ORG     ?= $(shell basename $(realpath ..))
PKGS    := $(shell go list ./... | grep -v /vendor/)

build:
	go build ${TARGETS}
.PHONY: build

generate:
	go generate ${PKGS}
.PHONY: generate

fmt:
	go fmt ${PKGS}
.PHONY: fmt

check:
	go vet ${PKGS}
.PHONY: check

test:
	go test -v ${PKGS} -cover -race -p=1
.PHONY: test

integration:
	bash test/integration/run.sh
.PHONY: integration

cross:
	gox -os '!freebsd !netbsd' -arch '!arm' -output "dist/{{.Dir}}_{{.OS}}_{{.Arch}}" -ldflags '-X main.Version=${TAG}' ${TARGETS}
.PHONY: cross

clean:
	rm dist/helmfile_*
.PHONY: clean

pristine: generate fmt
	git ls-files --exclude-standard --modified --deleted --others | diff /dev/null -
.PHONY: pristine

release: pristine cross
	@ghr -b ${BODY} -t ${GITHUB_TOKEN} -u ${ORG} -replace ${TAG} dist
.PHONY: release

image: cross
	docker build -t quay.io/${ORG}/helmfile:${TAG} .

run: image
	docker run --rm -it -t quay.io/${ORG}/helmfile:${TAG} sh

push: image
	docker push quay.io/${ORG}/helmfile:${TAG}

tools:
	go get -u github.com/tcnksm/ghr github.com/mitchellh/gox
.PHONY: tools

TAG  = $(shell git describe --tags --abbrev=0 HEAD)
LAST = $(shell git describe --tags --abbrev=0 HEAD^)
BODY = "`git log ${LAST}..HEAD --oneline --decorate` `printf '\n\#\#\# [Build Info](${BUILD_URL})'`"
