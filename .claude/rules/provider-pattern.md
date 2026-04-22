# Provider Pattern Rules

When modifying or adding cloud provider implementations:

## Interface Compliance

- All providers MUST implement the `Provider` interface (`pkg/cli/client/provider.go`)
- BYOC providers MUST implement `ProjectBackend` and embed `ByocBaseClient`
- Return `ErrNotImplemented` for methods that don't apply to a provider — never silently succeed

## Adding a New Provider Method

1. Add to the `Provider` interface in `client/provider.go`
2. Implement in `PlaygroundProvider` (playground.go)
3. Implement in `ByocBaseClient` if shared, or per-provider if not
4. Add to `MockProvider` (mock.go) for testability
5. Add corresponding Cobra command or flag in `cmd/cli/command/`

## BYOC Architecture

- Cloud-specific SDK calls go in `pkg/clouds/<provider>/` — NOT in the BYOC client
- The BYOC client in `pkg/cli/client/byoc/<provider>/` orchestrates using `pkg/clouds/`
- Shared BYOC logic stays in `baseclient.go` — per-provider overrides go in the provider directory
- The CD task runs in the USER'S cloud — never store or access user credentials
