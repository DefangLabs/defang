SHELL := bash

ifndef WITHOUT-NIX
ifneq (,$(shell command -v nix))
USING-NIX := true
override GO :=
override BUF :=
endif
endif

GO_VERSION := $(shell grep '^go ' go.mod | cut -d ' ' -f 2)


# Use Makes to auto-install all deps if/when needed (go & buf)
ifndef USING-NIX
R := $(abspath $(dir $(shell pwd)))
M := $R/.cache/makes
C := 606900040dd8132d2de51a0ea9302818a9826930
$(shell [[ -d $M ]] || ( \
  git clone -q https://github.com/makeplus/makes $M && \
  git -C $M reset -q --hard $C))

GO-VERSION ?= $(GO_VERSION).0

include $M/init.mk
include $M/go.mk
include $M/buf.mk
include $M/clean.mk
include $M/shell.mk

MAKES-CLEAN := defang

GOLANGCI-LINT := $(LOCAL-BIN)/golangci-lint
endif


# VERSION is the version we should download and use.
VERSION := $(shell git describe --match=NeVeRmAtCh --always --dirty)

PROTOS := \
  protos/io/defang/v1/fabric.pb.go \
  protos/io/defang/v1/defangv1connect/fabric.connect.go \

PROTOS-DEPS := \
  protos/io/defang/v1/fabric.proto \
  buf.gen.yaml \

BINARY_NAME := defang
GOFLAGS := -ldflags "-X main.version=$(VERSION)"
GOSRC := $(shell find . -name '*.go')

FAIL-RED := sed -e 's/\(--- FAIL.*\)/[0;31m\1[0m/g'

$(BINARY_NAME): $(GO) $(PROTOS) $(GOSRC) go.mod go.sum
	go build -o $@ $(GOFLAGS) ./cmd/cli

.PHONY: build
build: $(BINARY_NAME)

.PHONY: protos
protos: $(PROTOS)

$(PROTOS) &: $(BUF) $(PROTOS-DEPS)
	cd protos && buf lint
	buf generate protos

.PHONY: install
install: $(BINARY_NAME)
	install $(BINARY_NAME) "${HOME}/.local/bin/"

.PHONY: test
test: tidy $(GO) $(PROTOS)
	set -o pipefail && go test -short ./... | $(FAIL-RED)

.PHONY: integ
integ: $(GO) $(PROTOS)
	set -o pipefail && go test -v -tags=integration ./... | $(FAIL-RED)

.PHONY: linux-amd64
linux-amd64: $(GO) test
	CGO_ENABLED=0 \
	GOOS=linux \
	GOARCH=amd64 \
	go build -o $(BINARY_NAME) $(GOFLAGS) ./cmd/cli

.PHONY: tidy
tidy: $(GO)
	go mod tidy

defang_linux_amd64.zip: linux-amd64
	zip $@ $(BINARY_NAME)

defang-amd64: $(GO) test
	CGO_ENABLED=0 \
	GOOS=darwin \
	GOARCH=amd64 \
	go build -o $@ $(GOFLAGS) ./cmd/cli
	codesign -f -s "${MACOS_CERTIFICATE_NAME}" $@ \
	  --timestamp --options runtime

defang-arm64: $(GO) test
	CGO_ENABLED=0 \
	GOOS=darwin \
	GOARCH=arm64 \
	go build -o $@ $(GOFLAGS) ./cmd/cli
	codesign -f -s "${MACOS_CERTIFICATE_NAME}" $@ \
	  --timestamp --options runtime

defang_darwin.zip: defang-arm64 defang-amd64
	lipo -create -output $(BINARY_NAME) defang-arm64 defang-amd64
	codesign -f -s "${MACOS_CERTIFICATE_NAME}" $(BINARY_NAME) \
	  --timestamp --options runtime
	zip $@ $(BINARY_NAME)

.PHONY: zips
zips: defang_linux_amd64.zip defang_darwin.zip

.PHONY: no-diff
no-diff:
	@# check that there are no uncommitted changes
	git diff-index --quiet HEAD --

.PHONY: pull
pull:
	git pull

.PHONY: increment-patch-version
increment-patch-version:
	git tag $$( \
	  git tag -l 'v*' --sort=-v:refname | \
	  head -n1 | \
	  awk -F. '{$$NF = $$NF + 1;} 1' OFS=. \
	)

.PHONY: increment-minor-version
increment-minor-version:
	@git tag $$( \
		git tag -l 'v*' --sort=-v:refname | \
		head -n1 | \
		awk -F. '{$$(NF-1) = $$((NF-1)) + 1; $$NF = 0;} 1' OFS=. \
	)

.PHONY: increment-major-version
increment-major-version:
	@git tag $$( \
		git tag -l 'v*' --sort=-v:refname | \
		head -n1 | \
		awk -F. '{maj=substr($$1,2)+1; printf "v%d.0.0\n", maj}' \
	)

.PHONY: patch-release
patch-release: pull test no-diff increment-patch-version
	git push --follow-tags --tags

.PHONY: minor-release
minor-release: pull test no-diff increment-minor-version
	git push --follow-tags --tags

.PHONY: major-release
major-release: pull test no-diff increment-major-version
	git push --follow-tags --tags

.PHONY: release
release:
	@echo "To create a release, use one of the following targets:"
	@echo "  make patch-release   # Create a patch release (increments patch version)"
	@echo "  make minor-release   # Create a minor release (increments minor version)"
	@echo "  make major-release   # Create a major release (increments major version)"

.PHONY: lint
lint: $(GOLANGCI-LINT)
	@golangci-lint run || ( \
	  echo "Run 'make lint-fix' to try to fix the linting errors"; \
	  exit 1; \
	)

.PHONY: lint-fix
lint-fix: $(GOLANGCI-LINT)
	golangci-lint run --fix

GOLANGCI-LINT-INSTALL := \
  https://github.com/golangci/golangci-lint/raw/HEAD/install.sh

$(GOLANGCI-LINT): $(GO)
	curl -sSfL $(GOLANGCI-LINT-INSTALL) | \
	  sh -s -- -b $(LOCAL-BIN) v2.5.0

PROJECT_NAME := defang-cli

# DOCKER is the docker image repo we need to push to.
DOCKER_REPO:=defangio
DOCKER_IMAGE_NAME:=$(DOCKER_REPO)/$(PROJECT_NAME)

## Build and push multi-arch Docker images
.PHONY: push-images
push-images: login
	docker buildx build \
	  --platform linux/amd64,linux/arm64 \
	  --push \
	  -t $(DOCKER_IMAGE_NAME):$(VERSION) \
	  -t $(DOCKER_IMAGE_NAME):latest \
	  --build-arg VERSION=$(VERSION) \
	  --build-arg GO_VERSION=$(GO_VERSION) \
	  .

## Login to docker
.PHONY: login
login:
	@docker login

## Used for local testing of goreleaser
.PHONY: snapshot
snapshot:
	GORELEASER_ALLOW_DIRTY=true \
	GORELEASER_LOG=debug \
	goreleaser release \
	  --snapshot \
	  --clean \
	  --skip=publish,announce,sign,notarize,validate
