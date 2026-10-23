---
paths:
  - "src/pkg/cli/client/**/*.go"
  - "src/pkg/clouds/**/*.go"
---

# Provider Pattern Rules

- All providers implement `Provider` from `pkg/cli/client/provider.go`.
- BYOC providers implement `ProjectBackend` and use shared `ByocBaseClient` behavior where appropriate.
- Return `ErrNotImplemented` for unsupported provider methods; never silently succeed.
- Cloud SDK calls go in `pkg/clouds/<provider>/`, not in BYOC orchestration code.
- Shared BYOC logic stays in `baseclient.go`; per-provider behavior belongs in the provider directory.
- CD tasks run in the user's cloud. Never store or access user cloud credentials.
