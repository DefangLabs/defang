# Testing Rules

## Unit Tests

- Always run with `-short` flag: `cd src && go test -short ./...`
- Use table-driven tests for multiple scenarios
- Use `MockProvider` and `MockFabricClient` from `client/mock.go` — do not create ad-hoc mocks
- Test fixtures go in `src/testdata/` for compose files, or adjacent to the test file for other fixtures
- Test files are always adjacent: `foo.go` → `foo_test.go` (same package)

## Integration Tests

- Tag with `//go:build integration`
- Name as `*_integration_test.go`
- These require real cloud credentials — never run in standard CI or SAM environments without checking `get_credential_status` first
- Run via `cd src && make integ`

## Coverage

- CI enforces coverage must not decrease vs. main branch
- When adding new code paths, add corresponding test cases
