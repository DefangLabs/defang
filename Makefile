.PHONY: install-git-hooks
install-git-hooks:
	printf "#!/bin/sh\nmake pre-commit" > .git/hooks/pre-commit
	chmod +x .git/hooks/pre-commit
	printf "#!/bin/sh\nmake pre-push" > .git/hooks/pre-push
	chmod +x .git/hooks/pre-push

.PHONY: pre-commit
pre-commit:
	@if git diff --cached --name-only | grep -q '^src/'; then $(MAKE) -C src lint; fi

.PHONY: pre-push
pre-push: pkgs/npm/README.md src/README.md test-nix
	@$(MAKE) -C src test

.PHONY: setup
setup:
	go -C src mod tidy

pkgs/npm/README.md src/README.md: README.md
	@awk '/^## Develop Once, Deploy Anywhere\./{p=1} (/^## /||/^### /){if(p&&!/^## Develop Once, Deploy Anywhere\./){exit}} p' $< > src/README.md
	@awk '/^## Defang CLI/{p=1} (/^## /||/^### /){if(p&&!/^## Defang CLI/){exit}} p' $< >> src/README.md
	@awk '/^## Getting started/{p=1} (/^## /||/^### /){if(p&&!/^## Getting started/){exit}} p' $< >> src/README.md
	@awk '/^## Support/{p=1} (/^## /||/^### /){if(p&&!/^## Support/){exit}} p' $< >> src/README.md
	@awk '/^## Environment Variables/{p=1} (/^## /||/^### /){if(p&&!/^## Environment Variables/){exit}} p' $< >> src/README.md
	@cp src/README.md pkgs/npm/README.md
	@echo 'README files synced successfully. Please add any changes to your commit.'
	@false

.PHONY: test-nix
test-nix:
ifneq (,$(shell which nix))
	nix run .#defang-cli --extra-experimental-features flakes --extra-experimental-features nix-command
endif

.PHONY: clean distclean
clean distclean:
	$(MAKE) -C src $@
