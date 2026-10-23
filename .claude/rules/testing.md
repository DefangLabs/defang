---
paths:
  - "src/**/*.go"
  - "src/**/*_test.go"
---

# Testing Rules

- Run unit tests with `-short`: `cd src && go test -short ./...` or `cd src && make test`.
- Use table-driven tests for multiple scenarios.
- Test behavior boundaries, not implementation mechanics.
- Prefer small inline fixtures and helpers over fixture sprawl.
- Use `MockProvider` and `MockFabricClient` from `client/mock.go`; do not create ad-hoc mocks when existing mocks fit.
- Test files are adjacent to source: `foo.go` -> `foo_test.go`.
- Integration tests use `//go:build integration`, are named `*_integration_test.go`, and require real cloud credentials.
