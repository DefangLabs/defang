# Agents (Codex)

## Linting

- MUST run `cd src && make lint` before committing any Go changes
- MUST run `cd src && make lint-fix` to auto-fix lint issues when lint fails
- MUST NOT use `--no-verify` on git commits

## Testing

- MUST run `cd src && make test` before pushing
- Use `-short` flag for unit tests: `cd src && go test -short ./...`

## Git Hooks

Git hooks are configured via `core.hooksPath`. Run `make install-git-hooks` if hooks are not active.
