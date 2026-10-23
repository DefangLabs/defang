# Defang CLI Go Guidance

This file applies to Go work under `src/`.

## Package Boundaries

- `cmd/cli/command/` contains Cobra command wiring. Keep commands thin.
- `pkg/cli/compose/` owns Docker Compose loading, validation, normalization, and project-directory behavior.
- `pkg/session/` owns selected stack/session orchestration, not compose-project internals.
- `pkg/cli/client/` defines the Fabric and Provider interfaces.
- `pkg/cli/client/byoc/` orchestrates BYOC provider behavior.
- `pkg/clouds/<provider>/` contains low-level cloud SDK calls.
- `protos/` contains generated Connect-RPC protobuf code; regenerate after proto edits.

## Implementation Rules

- Prefer a narrow option passed into the owning package over duplicating path discovery or state reconstruction in a caller.
- Avoid exported API unless there is a stable cross-package caller.
- Name functions honestly: disk, network, goroutine, environment, and remote-state side effects should be visible from the name or call site.
- Treat zero values intentionally. Avoid pointers for optional strings unless three states matter.
- Do not silently swallow real errors. Return wrapped errors for filesystem, parsing, network, and provider failures.
- Missing optional files may be ignored only when that is the documented behavior.
- Avoid action at a distance. File-existence and working-directory behavior should be deterministic and tested at the owning layer.

## Tests

- Run unit tests with `-short`.
- Use adjacent `_test.go` files for package behavior.
- Put compose fixtures in `src/testdata/` or package `testdata/` only when inline setup would obscure the test.
- Fixture names should describe the behavior under test directly.
- Command-level tests live under `cmd/cli/command/`.
- Integration tests require `//go:build integration`, use `*_integration_test.go`, and require real cloud credentials.

## Provider Changes

- All providers implement `Provider` from `pkg/cli/client/provider.go`.
- BYOC providers implement `ProjectBackend` and embed or use `ByocBaseClient` where shared behavior applies.
- Return `ErrNotImplemented` for unsupported provider methods instead of silently succeeding.
- When adding provider interface methods, update Playground, BYOC/shared or per-provider implementations, and mocks in the same change.
- Never store or access user cloud credentials; BYOC CD tasks run in the user cloud.
