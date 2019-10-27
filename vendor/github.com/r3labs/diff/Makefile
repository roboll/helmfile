install:
	go install -v ${LDFLAGS}

test:
	@go test -v -cover ./...

cover:
	@go test -coverprofile cover.out

dev-deps:
	@go get -u github.com/stretchr/testify
	@go get github.com/alecthomas/gometalinter
	@gometalinter --install > /dev/null

lint:
	@gometalinter --vendor --disable-all --enable=errcheck --enable=golint --enable=vet ./...
