# Linting Rules

Before committing Go code changes:

1. Run `cd src && make lint` to check for lint errors
2. Run `cd src && make lint-fix` to auto-fix issues
3. Do NOT use `--no-verify` to skip pre-commit hooks
4. Do NOT commit code that fails `golangci-lint`

The PostToolUse hook auto-runs golangci-lint on edited Go files, but always verify with a full lint pass before committing.
