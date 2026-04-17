# Defang CLI

The Defang CLI is a Go-based command-line tool that deploys Docker Compose applications to multiple cloud providers (AWS, GCP, DigitalOcean) and the Defang Playground. It communicates with the Fabric backend via gRPC (Connect-RPC) and orchestrates BYOC (Bring Your Own Cloud) deployments where a CD task runs Pulumi inside the user's cloud account.

---

## Directory Structure

All Go source code lives under `src/`. The root `Makefile` delegates to `src/Makefile`.

```
src/
  cmd/cli/              Entry point (main.go) and cobra command definitions
    command/            Cobra commands (compose.go, config.go, stack.go, etc.)
  pkg/
    agent/              AI agent for `defang generate` (Genkit-based)
    auth/               OAuth/OpenAuth client (PKCE flow to Portal auth)
    cli/                Core CLI logic (compose up/down, config, tail, etc.)
      client/           Key interfaces and implementations:
        client.go         FabricClient interface (gRPC to Fabric)
        provider.go       Provider interface (cloud operations)
        grpc.go           GrpcClient (Connect-RPC implementation)
        playground.go     PlaygroundProvider (Defang-managed infra)
        mock.go           MockProvider (for tests)
        byoc/             BYOC base client and per-provider implementations:
          baseclient.go     Shared BYOC logic
          aws/              AWS BYOC provider
          gcp/              GCP BYOC provider
          do/               DigitalOcean BYOC provider
      compose/          Docker Compose loading, validation, normalization
    clouds/             Low-level cloud SDK wrappers (not client interfaces)
      aws/              AWS SDK calls (ECS, S3, Route53, etc.)
      gcp/              GCP SDK calls (Cloud Run, Artifact Registry, etc.)
      do/               DigitalOcean SDK calls
    mcp/                MCP (Model Context Protocol) server
    dns/                DNS resolution helpers
    logs/               Structured logging
    term/               Terminal I/O helpers
    track/              Analytics/telemetry
    types/              Shared types (TenantLabel, ETag, etc.)
  protos/               Protobuf definitions and generated code
    io/defang/v1/
      fabric.proto        Service definition (50+ RPCs)
      fabric.pb.go        Generated Go types
      defangv1connect/    Generated Connect-RPC client/server
  testdata/             Test fixtures (compose files for various scenarios)
pkgs/
  defang/               Nix package definition
  npm/                  npm wrapper package (downloads binary)
```

---

## Build, Test, and Lint

All commands run from `src/` (or use root `Makefile` which delegates).

```bash
# Build
cd src && make build          # Produces ./defang binary
cd src && go build -o defang ./cmd/cli

# Test (unit tests, skips integration)
cd src && make test           # Runs `go test -short ./...`
cd src && go test -short ./...

# Test a single package
cd src && go test -short ./pkg/cli/compose/...

# Integration tests (requires cloud credentials)
cd src && make integ          # Runs `go test -v -tags=integration ./...`

# Lint
cd src && make lint           # Runs golangci-lint v2.5.0
cd src && make lint-fix       # Auto-fix lint issues

# Tidy modules
cd src && make tidy           # go mod tidy

# Regenerate proto files
cd src && make protos         # Requires protoc, protoc-gen-go, protoc-gen-connect-go

# Install locally
cd src && make install        # Installs to ~/.local/bin/
```

---

## Key Abstractions

### FabricClient Interface (`pkg/cli/client/client.go`)

Defines the gRPC contract with the Fabric backend. Implemented by `GrpcClient`. Methods include `WhoAmI`, `Deploy`, `GetVersions`, `Token`, `GenerateCompose`, etc.

### Provider Interface (`pkg/cli/client/provider.go`)

Defines cloud-provider operations. Three implementations:
- **PlaygroundProvider** -- delegates everything to Fabric (Defang-managed)
- **BYOC AWS** (`client/byoc/aws/`) -- runs CD tasks via ECS
- **BYOC GCP** (`client/byoc/gcp/`) -- runs CD tasks via Cloud Build/Run
- **BYOC DO** (`client/byoc/do/`) -- DigitalOcean App Platform

Key methods: `Deploy`, `GetServices`, `Subscribe`, `QueryLogs`, `SetUpCD`, `CdCommand`.

### BYOC Pattern

BYOC providers share logic via `ByocBaseClient` (`client/byoc/baseclient.go`). Each provider implements `ProjectBackend` for cloud-specific operations. The CD task is a container that runs **in the user's cloud**, containing Pulumi. Defang never sees user credentials.

### Loader Interface (`pkg/cli/client/provider.go`)

Handles loading and parsing Docker Compose projects. The `compose` package (`pkg/cli/compose/`) implements validation, normalization, and fixup of compose files.

---

## Code Conventions

### Error Handling

- Wrap errors with context: `fmt.Errorf("doing X: %w", err)`
- User-facing errors must include remediation steps
- Custom error types for actionable errors (e.g., `ErrMultipleProjects` suggests `--project-name` flag)
- Use `ErrNotImplemented` for unimplemented provider methods

### Naming

- Packages use short, lowercase names (`cli`, `byoc`, `dns`)
- Interfaces end with purpose, not "Interface" (`FabricClient`, `Provider`, `Loader`)
- Test files are adjacent to source: `foo.go` / `foo_test.go`
- Integration tests use build tag: `//go:build integration`

### Logging and Output

- Use `pkg/term` for user-facing output (supports color, spinners)
- Use `log/slog` for structured debug logging
- Analytics via `pkg/track`

### Commit Messages

Follow conventional commits: `<type>(<scope>): <subject>`

Types: `feat`, `fix`, `docs`, `refactor`, `test`, `chore`

Scopes are optional; common ones: `aws`, `gcp`, `deps`

Examples from recent history:
```
fix: correct typo in deployment description in estimate summaries
feat: add --force to 'cd setup' to allow CD downgrades
fix(aws): use mutex for deployment-etag cache
chore(deps): bump github.com/docker/cli
```

---

## Testing Strategy

### Unit Tests

- Run with `-short` flag to skip integration tests
- Table-driven tests are preferred
- Use `MockProvider` and `MockFabricClient` (in `client/mock.go`) for testing CLI logic
- Test fixtures in `src/testdata/` (compose files for various scenarios)
- Command-level tests in `cmd/cli/command/` with `_test.go` files

### Integration Tests

- Tagged with `//go:build integration`
- Named `*_integration_test.go`
- Require cloud credentials (AWS, GCP)
- Run via `make integ`

### Coverage

- CI enforces coverage must not decrease vs. main branch
- Coverage report uploaded as artifact on every push

---

## Protobuf / gRPC Workflow

The CLI uses **Connect-RPC** (not standard gRPC) over HTTP/2.

Proto source: `src/protos/io/defang/v1/fabric.proto`

To regenerate after proto changes:
```bash
cd src && make protos
```

This requires: `protoc`, `protoc-gen-go`, `protoc-gen-connect-go`

CI verifies proto files are up to date. If you change `fabric.proto`, regenerate and commit the generated files.

The proto file is shared with `defang-mvp` (Fabric backend). Coordinate proto changes across both repos.

---

## CI/CD Pipeline

### Primary Workflow: `.github/workflows/go.yml`

Triggered on every push to any branch and on tags.

**Jobs:**
1. **go-test** -- Unit tests, coverage check (must not regress), module tidy check, proto freshness check, cross-platform builds (macOS, Windows)
2. **nix-shell-test** -- Validates Nix build, auto-updates `vendorHash` if needed
3. **go-playground-test** -- Sanity tests against Defang Playground (login, config, compose up/down)
4. **build-and-sign** (tags/main only) -- GoReleaser Pro builds for Linux/Windows, Azure Trusted Signing
5. **build-and-sign-mac** (tags/main only) -- GoReleaser Pro for macOS, Apple code signing + notarization
6. **go-release** (tags only) -- Merges platform builds, creates GitHub Release
7. **post-release** -- Triggers docs autodoc, Homebrew formula update, publishes npm package
8. **push-docker** -- Multi-arch Docker image push

### Other Workflows

- **smoketests.yml** -- Triggers defang-mvp smoke tests on main push
- **nightly.yml** -- Nightly builds
- **install.yml** -- Installation script tests

### Release Process

Releases are tag-driven. Create a semver tag to trigger the full release pipeline:
```bash
make patch-release   # Bumps patch version, pushes tag
make minor-release   # Bumps minor version, pushes tag
make major-release   # Bumps major version, pushes tag
```

---

## SAM (Simple Agent Manager) Integration

When running inside SAM (detected by `$SAM_WORKSPACE_ID` environment variable):

### Ephemeral Environment

SAM agent environments are **ephemeral**. Unpushed work is lost when the session ends. Push frequently:
- After each logical unit of work (new file, passing tests)
- Before running long operations that might time out
- Always before completing a task

### Task Status Updates

Call `update_task_status` at these milestones:
- After initial code research / understanding the problem
- After implementing the solution
- After tests pass
- Before creating the PR

### Cross-Repo Coordination

Use `dispatch_task` when changes span repos:
- **Proto changes** require updates in both `defang` (CLI) and `defang-mvp` (Fabric backend)
- **New CLI commands** may need corresponding Fabric RPCs
- **New provider features** may need Pulumi updates in `defang-mvp`

### Before Modifying Shared Interfaces

Call `list_project_agents` before changing:
- `fabric.proto` -- defines the CLI-to-Fabric contract
- `FabricClient` interface (`pkg/cli/client/client.go`)
- `Provider` interface (`pkg/cli/client/provider.go`)
- `ByocBaseClient` (`pkg/cli/client/byoc/baseclient.go`)

These are consumed by multiple providers and the Fabric backend.

### CI Verification

Call `get_ci_status` after pushing to verify:
- Unit tests pass
- Coverage has not regressed
- Proto files are up to date
- Cross-platform builds succeed

### Capturing Ideas

Use `create_idea` for improvements discovered during work:
- Missing test coverage for a code path
- Error messages that lack remediation steps
- Potential refactoring opportunities
- Documentation gaps

### SAM Workflow Summary

```
1. Research  -> update_task_status("in_progress", "Researching codebase")
2. Plan      -> list_project_agents (if touching shared interfaces)
3. Implement -> push early, push often
4. Test      -> cd src && make test
5. Push      -> git push
6. Verify    -> get_ci_status
7. PR        -> gh pr create
8. Complete  -> update_task_status("done", "PR created: <url>")
```
