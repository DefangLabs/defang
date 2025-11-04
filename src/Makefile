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
install: $(BINARY_NAME) test
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

.PHONY: version
version:
	git tag $$( \
	  git tag -l 'v*' --sort=-v:refname | \
	  head -n1 | \
	  awk -F. '{$$NF = $$NF + 1;} 1' OFS=. \
	)

.PHONY: release
release: pull test no-diff version
	git push --follow-tags --tags

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

DOCKER_IMAGE_ARM64:=$(DOCKER_IMAGE_NAME):arm64-$(VERSION)
DOCKER_IMAGE_AMD64:=$(DOCKER_IMAGE_NAME):amd64-$(VERSION)

.PHONY: image-amd64
image-amd64:
	docker build \
	  --platform linux/amd64 \
	  -t ${PROJECT_NAME} \
	  -t $(DOCKER_IMAGE_AMD64) \
	  --build-arg VERSION=$(VERSION) \
	  --build-arg GO_VERSION=$(GO_VERSION) \
	  .

.PHONY: image-arm64
image-arm64:
	docker build \
	  --platform linux/arm64 \
	  -t ${PROJECT_NAME} \
	  -t $(DOCKER_IMAGE_ARM64) \
	  --build-arg VERSION=$(VERSION) \
	  --build-arg GO_VERSION=$(GO_VERSION) \
	  .

## Build all docker images and manifest
.PHONY: images
images: image-amd64 image-arm64

## Push all docker images
.PHONY: push-images
push-images: images login
	docker push $(DOCKER_IMAGE_AMD64)
	docker push $(DOCKER_IMAGE_ARM64)
	docker manifest create --amend \
	  $(DOCKER_IMAGE_NAME):$(VERSION) \
	  $(DOCKER_IMAGE_AMD64) \
	  $(DOCKER_IMAGE_ARM64)
	docker manifest create --amend \
	  $(DOCKER_IMAGE_NAME):latest \
	  $(DOCKER_IMAGE_AMD64) \
	  $(DOCKER_IMAGE_ARM64)
	docker manifest push --purge \
	  $(DOCKER_IMAGE_NAME):$(VERSION)
	docker manifest push --purge \
	  $(DOCKER_IMAGE_NAME):latest

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

# ==============================================================================
# Genkit Evaluation System
# ==============================================================================

# Evaluation directories
EVAL_DIR := pkg/agent/evaluation
TESTDATA_DIR := $(EVAL_DIR)/datasets
GENKIT_CMD_DIR := $(EVAL_DIR)/cmd

# Configurable parameters
RUNS ?= 10
EVAL_OUTPUT_FILE_PATTERN := $$(basename $$dataset .json)_results.json
EVAL_SUMMARY_FILE ?= current_evaluation.json

# Dynamic output directories based on branch and commit
GIT_BRANCH_RAW := $(shell git rev-parse --abbrev-ref --symbolic-full-name @{u} 2>/dev/null | sed 's|origin/||' || git rev-parse --abbrev-ref HEAD)
GIT_BRANCH := $(shell echo "$(GIT_BRANCH_RAW)" | sed 's/\//-/g')
GIT_COMMIT := $(shell git rev-parse --short=6 HEAD)
EVAL_ROOT_NAME_DIR := $(EVAL_DIR)/testrun-$(GIT_BRANCH)
EVAL_BASE_DIR := $(EVAL_ROOT_NAME_DIR)-$(GIT_COMMIT)
RUN_TIMESTAMP := $(shell date +%Y%m%d-%H%M%S)
EVAL_OUTPUT_DIR := $(EVAL_BASE_DIR)/run-$(RUN_TIMESTAMP)

# Dataset discovery (exclude output files)
EVAL_DATASETS := $(filter-out $(TESTDATA_DIR)/evaluation_results.json, $(wildcard $(TESTDATA_DIR)/*.json))

# Server management targets
.PHONY: genkit-start
genkit-start: ## Start the genkit server for evaluations
	@echo "Starting Genkit server..."
	cd $(PWD)/$(GENKIT_CMD_DIR) && genkit start -- go run .

.PHONY: genkit-kill
genkit-kill: ## Kill any running genkit processes
	@echo "Killing any existing genkit processes..."
	@pkill -f "genkit start" || true
	@pkill -f "go run.*main.go" || true
	@sleep 2
	@echo "Genkit processes stopped."

.PHONY: genkit-restart
genkit-restart: genkit-kill genkit-start ## Kill existing and start fresh genkit server

.PHONY: genkit-check-server
genkit-check-server: ## Check if genkit server is running
	@if ! (curl -s http://localhost:4000 > /dev/null 2>&1 || curl -s http://localhost:4033 > /dev/null 2>&1 || curl -s http://localhost:4034 > /dev/null 2>&1); then \
		echo "Error: Genkit server is not running."; \
		echo ""; \
		echo "Please start the server in a separate terminal:"; \
		echo "  Terminal 1: cd $(PWD) && make genkit-start"; \
		echo ""; \
		echo "Then run evaluations from this terminal:"; \
		echo "  Terminal 2: cd $(PWD) && make genkit-evaluate"; \
		echo ""; \
		exit 1; \
	else \
		echo " Genkit server detected and running."; \
	fi

# Evaluation execution targets
.PHONY: genkit-evaluate
genkit-evaluate: genkit-check-server ## Run all genkit evaluations (requires genkit server to be running)
	@echo "Running all evaluations ($(RUNS) runs per dataset)..."
	@echo "Output directory: $(EVAL_OUTPUT_DIR)"
	@mkdir -p $(EVAL_OUTPUT_DIR)
	@for run in $$(seq 1 $(RUNS)); do \
		echo "Starting run $$run/$(RUNS)..."; \
		for dataset in $(EVAL_DATASETS); do \
			echo "  Running evaluation: $$dataset (run $$run)"; \
			cd $(PWD)/$(GENKIT_CMD_DIR) && genkit eval:flow defang-cli \
				--input "../datasets/$$(basename $$dataset)" \
				--output "$(PWD)/$(EVAL_OUTPUT_DIR)/$(EVAL_OUTPUT_FILE_PATTERN)" || true; \
		done; \
	done
	@echo "Generating evaluation summary..."
	@cd $(PWD)/$(EVAL_DIR)/tools/summarizer && go run . \
		-path ../../testrun-$(GIT_BRANCH)-$(GIT_COMMIT) \
		-output ../../$(EVAL_SUMMARY_FILE)
	@echo "Summary saved to $(EVAL_DIR)/$(EVAL_SUMMARY_FILE)"

.PHONY: genkit-summarize
genkit-summarize: ## Generate summary of current evaluation results
	@echo "Generating evaluation summary for $(EVAL_BASE_DIR)..."
	@cd $(PWD)/$(EVAL_DIR)/tools/summarizer && go run . \
		-path ../../testrun-$(GIT_BRANCH)-$(GIT_COMMIT) \
		-output ../../$(EVAL_SUMMARY_FILE)
	@echo "Summary saved to $(EVAL_DIR)/$(EVAL_SUMMARY_FILE)"

# Utility targets
.PHONY: genkit-list-datasets
genkit-list-datasets: ## List all available evaluation datasets
	@echo "Available evaluation datasets:"
	@for dataset in $(EVAL_DATASETS); do echo "  $$(basename $$dataset)"; done

.PHONY: genkit-clean
genkit-clean: ## Clean branch evaluation results
	@echo "Cleaning branch evaluation results: $(EVAL_BASE_DIR)"
	@rm -rf $(PWD)/$(GENKIT_CMD_DIR)/.genkit/evals/*.json
	@rm -rf $(EVAL_ROOT_NAME_DIR)-*
	@echo "Cleanup completed."

# Help and documentation
.PHONY: genkit-help
genkit-help: ## Show genkit evaluation help
	@echo "=== Genkit Evaluation System ==="
	@echo ""
	@echo "Server Management:"
	@echo "  genkit-start          Start the genkit server (run in separate terminal)"
	@echo "  genkit-kill           Kill any running genkit processes"
	@echo "  genkit-restart        Kill existing and start fresh genkit server"
	@echo ""
	@echo "Evaluation Execution:"
	@echo "  genkit-evaluate       Run all evaluations (default: $(RUNS) runs per dataset)"
	@echo ""
	@echo "Utilities:"
	@echo "  genkit-clean          Clean current branch/commit evaluation results"
	@echo "  genkit-help           Show this help message"
	@echo ""
	@echo "Configurable Parameters:"
	@echo "  RUNS=N                           Number of runs per dataset (default: $(RUNS))"
	@echo "  EVAL_OUTPUT_FILE_PATTERN=pattern Output filename pattern (default: dataset_results.json)"
	@echo "  EVAL_SUMMARY_FILE=filename       Summary output filename (default: $(EVAL_SUMMARY_FILE))"
	@echo ""
	@echo "Directory Structure:"
	@echo "  Base directory: $(EVAL_BASE_DIR)"
	@echo "  Current run:    $(EVAL_OUTPUT_DIR)"
	@echo ""
	@echo "Workflow (Two Terminal Setup):"
	@echo "  Terminal 1: make genkit-start    # Keep running"
	@echo "  Terminal 2: make genkit-evaluate # Run evaluations"
	@echo ""
	@echo "Usage Examples:"
	@echo "  make genkit-evaluate                                    # Default 10 runs"
	@echo "  make genkit-evaluate RUNS=5                             # Custom run count"
	@echo "  make genkit-evaluate EVAL_SUMMARY_FILE=my_results.json  # Custom summary file"
	@echo ""
	@echo "Troubleshooting:"
	@echo "  - If evaluations fail: make genkit-restart (in Terminal 1)"
	@echo "  - Clean old results:   make genkit-clean"
	@echo ""
