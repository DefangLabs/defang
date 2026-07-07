---
paths:
  - "src/**/*.go"
---

# Linting Rules

- Before committing Go code changes, run `cd src && make lint`.
- When lint fails, run `cd src && make lint-fix`, then rerun lint.
- Do not use `--no-verify` to skip pre-commit hooks.
- Do not commit code that fails `golangci-lint`.
