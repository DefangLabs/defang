.PHONY: install-git-hooks
install-git-hooks: $(BINARY_NAME)
	echo "#!/bin/sh\nmake pre-commit" > .git/hooks/pre-commit
	echo "#!/bin/sh\nmake pre-push" > .git/hooks/pre-push

.PHONY: pre-commit
pre-commit:
	cd src && go fmt ./...

.PHONY: pre-push
pre-push:
	make lint
	make -C src test

.PHONY: pre-commit
pre-commit:
	make lint

.PHONY: lint
lint:
	cd src && ../.bin/golangci-lint run

.PHONY: install-golangci-lint
install-golangci-lint:
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b .bin v1.61.0

.PHONY: setup
setup: install-git-hooks install-golangci-lint
