.PHONY: install-git-hooks
install-git-hooks: $(BINARY_NAME)
	echo "#!/bin/sh\nmake pre-commit" > .git/hooks/pre-commit
	echo "#!/bin/sh\nmake pre-push" > .git/hooks/pre-push

.PHONY: pre-commit
pre-commit:
	cd src && go fmt ./...

.PHONY: pre-push
pre-push:
	make -C src test

.PHONY: setup
setup: install-git-hooks
