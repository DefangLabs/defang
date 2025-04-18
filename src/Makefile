# VERSION is the version we should download and use.
VERSION:=$(shell git describe --match=NeVeRmAtCh --always --dirty)

PROTOS := protos/io/defang/v1/fabric.pb.go protos/io/defang/v1/defangv1connect/fabric.connect.go

BINARY_NAME:=defang
GOFLAGS:=-ldflags "-X main.version=$(VERSION)"
GOSRC := $(shell find . -name '*.go')

$(BINARY_NAME): $(PROTOS) $(GOSRC) go.mod go.sum
	go build -o $@ $(GOFLAGS) ./cmd/cli

.PHONY: build
build: $(BINARY_NAME)

.PHONY: protos
protos: $(PROTOS)
$(PROTOS) &: protos/io/defang/v1/fabric.proto buf.gen.yaml
	cd protos && buf lint
	buf generate protos

.PHONY: install
install: $(BINARY_NAME) test
	install $(BINARY_NAME) "${HOME}/.local/bin/"

.PHONY: test
test: $(PROTOS)
	go mod tidy
	set -o pipefail ; go test -test.short ./... | sed -e 's/\(--- FAIL.*\)/[0;31m\1[0m/g'

.PHONY: integ
integ: $(PROTOS)
	set -o pipefail ; go test -v -tags=integration ./... | sed -e 's/\(--- FAIL.*\)/[0;31m\1[0m/g'

.PHONY: linux-amd64
linux-amd64: test
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o $(BINARY_NAME) $(GOFLAGS) ./cmd/cli

defang_linux_amd64.zip: linux-amd64
	zip $@ $(BINARY_NAME)

defang-amd64: test
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -o $@ $(GOFLAGS) ./cmd/cli
	codesign -f -s "${MACOS_CERTIFICATE_NAME}" $@ --timestamp --options runtime

defang-arm64: test
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o $@ $(GOFLAGS) ./cmd/cli
	codesign -f -s "${MACOS_CERTIFICATE_NAME}" $@ --timestamp --options runtime

defang_darwin.zip: defang-arm64 defang-amd64
	lipo -create -output $(BINARY_NAME) defang-arm64 defang-amd64
	codesign -f -s "${MACOS_CERTIFICATE_NAME}" $(BINARY_NAME) --timestamp --options runtime
	zip $@ $(BINARY_NAME)

.PHONY: zips
zips: defang_linux_amd64.zip defang_darwin.zip

.PHONY: no-diff
no-diff:
	git diff-index --quiet HEAD --       # check that there are no uncommitted changes

.PHONY: pull
pull:
	git pull

.PHONY: version
version:
	git tag $$(git tag -l 'v*' --sort=-v:refname | head -n1 | awk -F. '{$$NF = $$NF + 1;} 1' OFS=.)

.PHONY: release
release: pull test no-diff version
	git push --follow-tags --tags

.PHONY: lint
lint:
	@golangci-lint run || (echo "Run 'make lint-fix' to try to fix the linting errors" && exit 1)

.PHONY: lint-fix
lint-fix:
	golangci-lint run --fix
