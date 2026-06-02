# Agent Reference

This file holds longer reference material for agents. It is intentionally not imported by `AGENTS.md` or `CLAUDE.md`; read it on demand when architecture context is useful.

## Directory Structure

```text
src/
  cmd/cli/              Entry point and Cobra command definitions
    command/            Commands such as compose, config, stack
  pkg/
    agent/              AI agent for compose generation
    auth/               OAuth/OpenAuth client
    cli/                Core CLI logic
      client/           FabricClient, Provider, gRPC client, Playground provider, mocks
      client/byoc/      BYOC base client and per-provider implementations
      compose/          Compose loading, validation, normalization, fixup
    clouds/             Low-level cloud SDK wrappers
    dns/                DNS helpers
    logs/               Structured logging
    mcp/                MCP server
    term/               Terminal I/O helpers
    track/              Telemetry
    types/              Shared types
  protos/               Protobuf definitions and generated Connect-RPC code
  testdata/             Shared test fixtures
pkgs/
  defang/               Nix package definition
  npm/                  npm wrapper package
```

## Key Abstractions

### FabricClient

`pkg/cli/client/client.go` defines the gRPC contract with Fabric. `GrpcClient` implements it.

### Provider

`pkg/cli/client/provider.go` defines cloud-provider operations. Implementations include Playground and BYOC providers for AWS, GCP, and DigitalOcean.

### BYOC Pattern

BYOC providers share orchestration through `ByocBaseClient`. Each provider implements cloud-specific behavior through `ProjectBackend`. CD tasks run in the user's cloud account and contain Pulumi.

### Compose Loader

`pkg/cli/compose/` owns Docker Compose loading, validation, normalization, and fixup.

## Protobuf Workflow

The CLI uses Connect-RPC over HTTP/2. Proto source lives at `src/protos/io/defang/v1/fabric.proto`. After editing proto files, run `cd src && make protos` and commit generated files.

## Release and CI Notes

Primary workflow: `.github/workflows/go.yml`.

Expected checks include unit tests, coverage comparison, module tidy, proto freshness, Nix build validation, and platform builds. Releases are tag-driven through GoReleaser.

## SAM Workflow

When running inside SAM:

- Report meaningful milestones with `update_task_status`.
- Push before task completion because SAM workspaces are ephemeral.
- Use `dispatch_task` for cross-repo changes such as CLI plus Fabric proto updates.
- Use `create_idea` for follow-up work that should not block the current task.
