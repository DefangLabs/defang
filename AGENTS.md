# Defang CLI Agent Guide

Keep this file short. It is loaded at startup by coding agents, so it should contain only rules that matter for most tasks.

## Repository Basics

- This repository contains the Defang CLI, a Go command-line tool under `src/`.
- The root `Makefile` delegates to `src/Makefile`.
- Work from `src/` for Go commands unless a root command explicitly delegates there.
- For deeper architecture context, read `docs/agent-reference.md` on demand instead of expanding this file.

## Common Commands

- Build: `cd src && make build`
- Unit tests: `cd src && make test` or `cd src && go test -short ./...`
- Focused package test: `cd src && go test -short ./pkg/cli/compose/...`
- Lint: `cd src && make lint`
- Auto-fix lint: `cd src && make lint-fix`
- Tidy modules: `cd src && make tidy`
- Regenerate protos after editing `fabric.proto`: `cd src && make protos`

## Agent Enforcement

- Before committing Go changes, run `cd src && make lint`.
- If lint fails, run `cd src && make lint-fix`, then rerun lint.
- Before pushing Go changes, run `cd src && make test` unless the user explicitly scopes verification differently.
- Do not use `--no-verify` to bypass hooks.
- If a required tool is missing in the environment, say exactly what could not run.

## Go Change Quality

- Make the smallest coherent change that fixes the root problem.
- Put behavior in the package that owns the concept: commands orchestrate, session/stack code selects stacks, compose code loads compose projects, provider code handles cloud behavior.
- Avoid adding exported functions, options, or struct fields for one-off behavior.
- Do not split one invariant across packages unless that boundary already exists.
- Reuse existing Defang abstractions and standard library helpers before adding new helpers.
- Keep CLI commands thin: parse flags, construct existing managers, call domain logic, print results.
- Return real errors with context. Ignore missing optional files only when missing is the documented contract.
- Keep provider-generic code generic; provider-specific behavior belongs behind provider-specific implementations or explicit parameters.
- Comments should explain why, not restate what the code does.

## Testing Expectations

- Use table-driven tests for multiple scenarios.
- Test behavior boundaries, not implementation mechanics.
- Prefer small inline fixtures and helpers over fixture sprawl.
- Add tests for new behavior and important failure modes: not-found vs other errors, duplicate adds, missing returns, blocking behavior, provider mismatches, and optional-file handling.
- Use existing mocks such as `MockProvider` and `MockFabricClient` instead of ad-hoc mocks.

## Nested Guidance

- When changing Go code under `src/`, read `src/AGENTS.md` before editing.
- Claude Code also has path-scoped rules in `.claude/rules/` that load for matching files.
