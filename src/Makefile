# VERSION is the version we should download and use.
VERSION:=$(shell git describe --match=NeVeRmAtCh --always --dirty)
# DOCKER is the docker image repo we need to push to.
DOCKER:=532501343364.dkr.ecr.us-west-2.amazonaws.com
IMAGE:=fabric
DOCKER_IMAGE_NAME:=$(DOCKER)/$(IMAGE)

REGION:=us-west-2
AWS_PROFILE:=defang-sandbox

DOCKER_IMAGE_ARM64:=$(DOCKER_IMAGE_NAME):arm64-$(VERSION)
DOCKER_IMAGE_AMD64:=$(DOCKER_IMAGE_NAME):amd64-$(VERSION)

PROTOS := protos/io/defang/v1/fabric.pb.go protos/io/defang/v1/defangv1connect/fabric.connect.go

BINARY_NAME:=defang
GOFLAGS:=-ldflags "-X main.version=$(VERSION)"

$(BINARY_NAME): test
	go build -o $@ $(GOFLAGS) ./cmd/cli

$(PROTOS) &: protos/io/defang/v1/fabric.proto buf.gen.yaml
	buf generate protos

.PHONY: protos
protos: $(PROTOS)
	# buf lint protos
	npm -C cd run build-protos || true

.PHONY: install
install: $(BINARY_NAME)
	install $(BINARY_NAME) "$${GOPATH:-/usr/local}/bin/"

.PHONY: no-diff
no-diff:
	git diff-index --quiet HEAD -- .      # check that there are no uncommitted changes

.PHONY: test
test: protos
	go test -test.short ./...

.PHONY: integ
integ: protos
	AWS_SDK_LOAD_CONFIG=1 AWS_PROFILE=$(AWS_PROFILE) AWS_REGION=$(REGION) go test ./... -tags=integration

.PHONY: docker-arm64
docker-arm64: test
	docker build --build-arg NODE_AUTH_TOKEN=${NODE_AUTH_TOKEN} --file Dockerfile.$(IMAGE) --tag $(DOCKER_IMAGE_ARM64) --platform linux/arm64 .

.PHONY: docker-amd64
docker-amd64: test
	docker build --build-arg NODE_AUTH_TOKEN=${NODE_AUTH_TOKEN} --file Dockerfile.$(IMAGE) --tag $(DOCKER_IMAGE_AMD64) --platform linux/amd64 .

.PHONY: buildx
buildx: test ecr-login
	@docker buildx create --use --name=crossplat --node=crossplat && \
	docker buildx build \
		--build-arg NODE_AUTH_TOKEN=${NODE_AUTH_TOKEN} \
		--file Dockerfile.$(IMAGE) \
		--tag $(DOCKER_IMAGE_NAME):$(VERSION) \
		--platform linux/arm64,linux/amd64 \
		--output "type=image,push=false" \
		.

.PHONY: docker
docker: docker-arm64 docker-amd64

.PHONY: push-arm64
push-arm64: no-diff docker-arm64 ecr-login
	docker push $(DOCKER_IMAGE_ARM64)

.PHONY: push-amd64
push-amd64: no-diff docker-amd64 ecr-login
	docker push $(DOCKER_IMAGE_AMD64)

.PHONY: push
push: push-arm64 push-amd64
	docker manifest create --amend $(DOCKER_IMAGE_NAME):$(VERSION) $(DOCKER_IMAGE_AMD64) $(DOCKER_IMAGE_ARM64)
	docker manifest create --amend $(DOCKER_IMAGE_NAME):latest $(DOCKER_IMAGE_AMD64) $(DOCKER_IMAGE_ARM64)
	docker manifest push --purge $(DOCKER_IMAGE_NAME):$(VERSION)
	docker manifest push --purge $(DOCKER_IMAGE_NAME):latest

server: test
	go build ./cmd/server

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

.PHONY: ecr-login
ecr-login:
	aws ecr get-login-password --profile $(AWS_PROFILE) --region $(REGION) | docker login --username AWS --password-stdin $(DOCKER)
