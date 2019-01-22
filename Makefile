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
	env CGO_ENABLED=0 gox -os '!freebsd !netbsd' -arch '!arm' -output "dist/{{.Dir}}_{{.OS}}_{{.Arch}}" -ldflags '-X main.Version=${TAG}' ${TARGETS}
.PHONY: cross

static-linux:
	env CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "dist/helmfile_linux_amd64" -ldflags '-X main.Version=${TAG}' ${TARGETS}
.PHONY: linux

clean:
	rm dist/helmfile_*
.PHONY: clean

pristine: generate fmt
	git ls-files --exclude-standard --modified --deleted --others | diff /dev/null -
.PHONY: pristine

release: pristine cross
	@ghr -b ${BODY} -t ${GITHUB_TOKEN} -u ${ORG} -recreate ${TAG} dist
.PHONY: release

image:
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

release/minor:
	git checkout master
	git pull --rebase origin master
	bash -c 'if git branch | grep autorelease; then git branch -D autorelease; else echo no branch to be cleaned; fi'
	git checkout -b autorelease origin/master
	bash -c 'SEMTAG_REMOTE=origin hack/semtag final -s minor'

release/patch:
	git checkout master
	git pull --rebase origin master
	bash -c 'if git branch | grep autorelease; then git branch -D autorelease; else echo no branch to be cleaned; fi'
	git checkout -b autorelease origin/master
	bash -c 'SEMTAG_REMOTE=origin hack/semtag final -s patch'
