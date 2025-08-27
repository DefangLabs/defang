.PHONY: install-git-hooks
install-git-hooks:
	printf "#!/bin/sh\nmake pre-commit" > .git/hooks/pre-commit
	chmod +x .git/hooks/pre-commit
	printf "#!/bin/sh\nmake pre-push" > .git/hooks/pre-push
	chmod +x .git/hooks/pre-push

.PHONY: pre-commit
pre-commit:
	@if git diff --cached --name-only | grep -q '^src/'; then make -C src lint; fi

.PHONY: pre-push
pre-push: sync-readme
	@make -C src test

setup:
	go -C src mod tidy

.PHONY: sync-readme
sync-readme:
	@if git diff --name-only HEAD | grep -q '^README.md$$'; then \
	  awk '/^## Develop Once\. Deploy Anywhere\./{p=1} (/^## /||/^### /){if(p&&!/^## Develop Once\. Deploy Anywhere\./){exit}} p' README.md > src/README.md; \
	  awk '/^## Defang CLI/{p=1} (/^## /||/^### /){if(p&&!/^## Defang CLI/){exit}} p' README.md >> src/README.md; \
	  awk '/^## Getting started/{p=1} (/^## /||/^### /){if(p&&!/^## Getting started/){exit}} p' README.md >> src/README.md; \
	  awk '/^## Support/{p=1} (/^## /||/^### /){if(p&&!/^## Support/){exit}} p' README.md >> src/README.md; \
	  awk '/^## Environment Variables/{p=1} (/^## /||/^### /){if(p&&!/^## Environment Variables/){exit}} p' README.md >> src/README.md; \
	  echo 'src/README.md updated.'; \
	else \
	  echo 'README.md unchanged, skipping sync.'; \
	fi
