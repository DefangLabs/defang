.PHONY: install-git-hooks
install-git-hooks: $(BINARY_NAME)
	echo "#!/bin/sh\nmake pre-commit" > .git/hooks/pre-commit

.PHONY: pre-commit
pre-commit:
	cd src && go fmt ./...

.PHONY: setup
setup: install-git-hooks
