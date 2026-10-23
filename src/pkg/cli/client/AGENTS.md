# Client and Provider Guidance

This file applies to `src/pkg/cli/client/` and nested BYOC provider code.

## Provider Interface

- `provider.go` defines the provider contract. Interface additions must update all implementations and mocks in the same change.
- Return `ErrNotImplemented` for unsupported provider methods instead of silently succeeding.
- Keep provider-generic orchestration in shared code and provider-specific behavior in provider-specific files.

## BYOC Pattern

- BYOC providers should use shared `ByocBaseClient` behavior when the behavior is genuinely common.
- Per-provider overrides belong under the provider directory.
- Cloud SDK calls belong in `pkg/clouds/<provider>/`, not directly in BYOC orchestration code.
- Never store or access user cloud credentials; CD tasks run in the user's cloud.

## Tests

- Update `MockProvider` and related mocks when interfaces change.
- Prefer explicit not-found vs. other-error test cases for provider behavior.
