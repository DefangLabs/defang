# Cloud SDK Wrapper Guidance

This file applies to `src/pkg/clouds/`.

## Ownership

- This tree contains low-level cloud SDK wrappers.
- Keep orchestration and CLI behavior outside this tree; those belong in `pkg/cli/client` or command packages.
- Provider-specific behavior should stay under the relevant provider directory.

## Error Handling

- Preserve cloud-provider error detail while wrapping errors with operation context.
- Distinguish not-found, permission, quota, and transient failures when the caller can act differently.
- Do not silently succeed on unsupported operations.

## Credentials

- Never persist user cloud credentials.
- Avoid logging secrets, tokens, account credentials, or raw SDK request bodies that may contain secrets.
