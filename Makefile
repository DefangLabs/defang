.PHONY: install-git-hooks
install-git-hooks:
	printf "#!/bin/sh\nmake pre-commit" > .git/hooks/pre-commit
	chmod +x .git/hooks/pre-commit
	printf "#!/bin/sh\nmake pre-push" > .git/hooks/pre-push
	chmod +x .git/hooks/pre-push

.PHONY: pre-commit
pre-commit:
	make lint-fix

.PHONY: pre-push
pre-push:
	make -C src test

.PHONY: lint
lint:
	cd src && golangci-lint run

.PHONY: lint-fix
lint-fix:
	cd src && golangci-lint run --fix
