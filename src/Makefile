SHELL := bash

# VERSION is the version we should download and use.
VERSION:=$(shell git describe --match=NeVeRmAtCh --always --dirty)

PROTOS := protos/io/defang/v1/fabric.pb.go protos/io/defang/v1/defangv1connect/fabric.connect.go

BINARY_NAME:=defang
GOFLAGS:=-ldflags "-X main.version=$(VERSION)"
GOSRC := $(shell find . -name '*.go')
GO_VERSION := $(shell grep '^go ' go.mod | cut -d ' ' -f 2)

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
	set -o pipefail ; go test -short ./... | sed -e 's/\(--- FAIL.*\)/[0;31m\1[0m/g'

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

PROJECT_NAME := defang-cli

# DOCKER is the docker image repo we need to push to.
DOCKER_REPO:=defangio
DOCKER_IMAGE_NAME:=$(DOCKER_REPO)/$(PROJECT_NAME)

DOCKER_IMAGE_ARM64:=$(DOCKER_IMAGE_NAME):arm64-$(VERSION)
DOCKER_IMAGE_AMD64:=$(DOCKER_IMAGE_NAME):amd64-$(VERSION)

.PHONY: image-amd64
image-amd64:
	docker build --platform linux/amd64 -t ${PROJECT_NAME} -t $(DOCKER_IMAGE_AMD64) --build-arg VERSION=$(VERSION) --build-arg GO_VERSION=$(GO_VERSION) .

.PHONY: image-arm64
image-arm64:
	docker build --platform linux/arm64 -t ${PROJECT_NAME} -t $(DOCKER_IMAGE_ARM64) --build-arg VERSION=$(VERSION) --build-arg GO_VERSION=$(GO_VERSION) .

.PHONY: images
images: image-amd64 image-arm64 ## Build all docker images and manifest

.PHONY: push-images
push-images: images login ## Push all docker images
	docker push $(DOCKER_IMAGE_AMD64)
	docker push $(DOCKER_IMAGE_ARM64)
	docker manifest create --amend $(DOCKER_IMAGE_NAME):$(VERSION) $(DOCKER_IMAGE_AMD64) $(DOCKER_IMAGE_ARM64)
	docker manifest create --amend $(DOCKER_IMAGE_NAME):latest $(DOCKER_IMAGE_AMD64) $(DOCKER_IMAGE_ARM64)
	docker manifest push --purge $(DOCKER_IMAGE_NAME):$(VERSION)
	docker manifest push --purge $(DOCKER_IMAGE_NAME):latest

.PHONY: login
login: ## Login to docker
	@docker login

.PHONY: snapshot
snapshot: ## Used for local testing of goreleaser
	GORELEASER_ALLOW_DIRTY=true GORELEASER_LOG=debug \
	goreleaser release --snapshot --clean \
	--skip=publish,announce,sign,notarize,validate

# Genkit evaluation targets
EVAL_DIR := pkg/agent/evaluation
TESTDATA_DIR := $(EVAL_DIR)/datasets
GENKIT_CMD_DIR := $(EVAL_DIR)/cmd

# Generate dynamic output directory based on branch and commit
GIT_BRANCH_RAW := $(shell git rev-parse --abbrev-ref --symbolic-full-name @{u} 2>/dev/null | sed 's|origin/||' || git rev-parse --abbrev-ref HEAD)
GIT_BRANCH := $(shell echo "$(GIT_BRANCH_RAW)" | sed 's/\//-/g')
GIT_COMMIT := $(shell git rev-parse --short=6 HEAD)
EVAL_BASE_DIR := $(EVAL_DIR)/testrun-$(GIT_BRANCH)-$(GIT_COMMIT)
# Generate unique run directory with timestamp
RUN_TIMESTAMP := $(shell date +%Y%m%d-%H%M%S)
EVAL_OUTPUT_DIR := $(EVAL_BASE_DIR)/run-$(RUN_TIMESTAMP)

# List of evaluation datasets (excluding evaluation_results.json which is output)
EVAL_DATASETS := $(filter-out $(TESTDATA_DIR)/evaluation_results.json, $(wildcard $(TESTDATA_DIR)/*.json))

.PHONY: genkit-start
genkit-start: ## Start the genkit server for evaluations
	@echo "Starting Genkit server..."
	cd $(PWD)/$(GENKIT_CMD_DIR) && genkit start -- go run .

.PHONY: genkit-kill
genkit-kill: ## Kill any running genkit processes
	@echo "Killing any existing genkit processes..."
	@pkill -f "genkit start" || true
	@sleep 2

.PHONY: genkit-restart
genkit-restart: genkit-kill genkit-start ## Kill existing and start fresh genkit server

.PHONY: genkit-check-server
genkit-check-server: ## Check if genkit server is running
	@if ! (curl -s http://localhost:4000 > /dev/null 2>&1 || curl -s http://localhost:4033 > /dev/null 2>&1 || curl -s http://localhost:4034 > /dev/null 2>&1); then \
		echo "Error: Genkit server is not running."; \
		echo "Please start the server in another terminal with: make genkit-start"; \
		echo "Make sure to keep the server running while evaluations are running."; \
		exit 1; \
	else \
		echo "Genkit server detected."; \
	fi

.PHONY: genkit-eval-all
genkit-eval-all: genkit-check-server ## Run all genkit evaluations (requires genkit server to be running)
	@echo "Running all evaluations..."
	@echo "Output directory: $(EVAL_OUTPUT_DIR)"
	@mkdir -p $(EVAL_OUTPUT_DIR)
	@for dataset in $(EVAL_DATASETS); do \
		echo "Running evaluation: $$dataset"; \
		cd $(PWD)/$(GENKIT_CMD_DIR) && genkit eval:run ../datasets/$$(basename $$dataset) --output "../$(EVAL_BASE_DIR)/run-$(RUN_TIMESTAMP)/$$(basename $$dataset .json)_results.json" || true; \
	done
	@echo "Moving results from .genkit/evals/ to $(EVAL_OUTPUT_DIR)/"
	@if [ -d "$(PWD)/$(GENKIT_CMD_DIR)/.genkit/evals" ]; then \
		mv $(PWD)/$(GENKIT_CMD_DIR)/.genkit/evals/*.json $(EVAL_OUTPUT_DIR)/ 2>/dev/null || true; \
	fi


.PHONY: genkit-list-datasets
genkit-list-datasets: ## List all available evaluation datasets
	@echo "Available evaluation datasets:"
	@for dataset in $(EVAL_DATASETS); do echo "  $$(basename $$dataset)"; done

.PHONY: genkit-clean
genkit-clean: ## Clean all evaluation results
	@echo "Cleaning all evaluation results..."
	rm -rf $(EVAL_DIR)/testrun-*

.PHONY: genkit-clean-current
genkit-clean-current: ## Clean current branch evaluation results
	@echo "Cleaning current branch evaluation results: $(EVAL_BASE_DIR)"
	rm -rf $(EVAL_BASE_DIR)


.PHONY: genkit-help
genkit-help: ## Show genkit evaluation help
	@echo "Genkit Evaluation Targets:"
	@echo "  genkit-start          Start the genkit server (run in separate terminal)"
	@echo "  genkit-kill           Kill any running genkit processes"
	@echo "  genkit-restart        Kill existing and start fresh genkit server"
	@echo "  genkit-check-server   Check if genkit server is running"
	@echo "  genkit-eval-all       Run all evaluations (checks server first)"
	@echo "  genkit-list-datasets  List all available datasets"
	@echo "  genkit-clean          Clean all evaluation results"
	@echo "  genkit-clean-current  Clean current branch/commit evaluation results"
	@echo ""
	@echo "Directory Structure:"
	@echo "  Base directory: $(EVAL_BASE_DIR)"
	@echo "  Current run:    $(EVAL_OUTPUT_DIR)"
	@echo ""
	@echo "IMPORTANT: You MUST keep the genkit server running in a separate terminal!"
	@echo "Each run creates a unique timestamped directory to organize results."
	@echo ""
	@echo "Usage Examples:"
	@echo "  Terminal 1: make genkit-start    (keep running)"
	@echo "  Terminal 2: make genkit-eval-all"
	@echo ""
	@echo "If evals fail, try: make genkit-restart (in Terminal 1)"
