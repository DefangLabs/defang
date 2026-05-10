# Scaleway Deployment Work Log

## Goal
Deploy the `nextjs-postgres` sample to Scaleway using the Scaleway provider PRs:
- **CLI PR #2105**: `feat/scaleway-byoc` branch in `DefangLabs/defang`
- **Pulumi PR #234**: `sam/httpsgithubcomdefanglabspulumi-defang-deep-dive-01kr1k` branch in `DefangLabs/pulumi-defang`

## Sample: nextjs-postgres
```yaml
services:
  app:
    build:
      context: ./app
      dockerfile: Dockerfile
    ports:
      - target: 3000, published: 3000, mode: ingress
    environment:
      POSTGRES_USER: postgres
      POSTGRES_PASSWORD: null  # resolved from config
      POSTGRES_HOST: database
      POSTGRES_PORT: 5432
      POSTGRES_DB: postgres
      POSTGRES_SSL: null
    depends_on: [database]
  database:
    image: postgres:16
    x-defang-postgres: true
    environment:
      POSTGRES_PASSWORD: null
    ports:
      - target: 5432, published: 5432, mode: host
```

---

## Blockers Identified (2026-05-09)

### 1. CD Task Uses Wrong Entry Point (CRITICAL)
**File**: `src/pkg/cli/client/byoc/scaleway/byoc.go:183`
**Issue**: Scaleway BYOC runs `node lib/index.js` — this is the old Node.js CD from defang-mvp, which is AWS-only.
**Fix**: Change to `/app/cd` (the Go CD binary from pulumi-defang/cd/), like Azure does.

### 2. CD Task Doesn't Detect Scaleway Provider (CRITICAL)
**File**: `pulumi-defang/cd/main.go` → `stackConfig()` function
**Issue**: `stackConfig()` checks for `AWS_REGION`, `GCP_PROJECT_ID`, `AZURE_SUBSCRIPTION_ID` but NOT Scaleway env vars (`SCW_DEFAULT_PROJECT_ID`). Falls through to error: "no cloud provider configured".
**Fix**: Add `SCW_DEFAULT_PROJECT_ID` detection → set `defang:provider = "scaleway"`.
Also: Add `"scaleway"` to `disable-default-providers` in `projectConfig()`.

### 3. Scaleway Provider Doesn't Support Image Building (CRITICAL for nextjs-postgres)
**File**: `pulumi-defang/provider/defangscaleway/project.go:165-167`
**Issue**: Rejects services with `build:` unless they also have `image:`. The nextjs-postgres `app` service has `build: ./app` without explicit `image:`.
**Context**: AWS uses CodeBuild, GCP uses Cloud Build. Scaleway has no equivalent build service.
**Options**:
  a. Build images inside the CD task container using Docker/Kaniko
  b. Use Scaleway Container Registry's build capabilities (if available)
  c. Require pre-built images (not viable for this sample)
**Decision**: TBD — need to research Scaleway's build options

### 4. POSTGRES_USER: "postgres" Is Reserved (MEDIUM)
**File**: `pulumi-defang/provider/defangscaleway/scaleway/postgres.go`
**Issue**: Scaleway rejects `postgres` as username (reserved). The sample has `POSTGRES_USER: postgres`.
**Fix**: The provider already defaults to `defang` when POSTGRES_USER is not set. But the sample explicitly sets it to `postgres`. Need to either:
  a. Auto-remap `postgres` → `defang` with a warning
  b. Let it fail and require the user to use a different username via config
**Decision**: Option (a) — auto-remap with warning, since we can't edit the sample

### 5. POSTGRES_HOST: "database" Won't Resolve on Scaleway
**Issue**: The app expects `POSTGRES_HOST=database` (Docker Compose service name). On Scaleway, managed Postgres has a different hostname (IP or Scaleway DNS name). The app needs the actual Scaleway Postgres endpoint.
**Context**: On AWS, the BYOC provider handles this via private DNS zones. On Scaleway, private networks are egress-only for containers — containers can't accept inbound private traffic.
**Fix**: The Pulumi provider needs to inject the correct POSTGRES_HOST value into the app service's environment. Need to check how other providers handle this.

---

## Environment Setup
- Go installed at `~/go/bin/go`
- CLI built from `feat/scaleway-byoc` branch
- Fabric auth works: `DEFANG_ACCESS_TOKEN=$DEFANG_TOKEN`
- **Missing**: Scaleway credentials (`SCW_ACCESS_KEY`, `SCW_SECRET_KEY`, `SCW_DEFAULT_PROJECT_ID`)

---

## Changes Made

### Session 1 (previous - 7a842ef5)
- Reviewed and renamed PRs
- Added `command` parameter to jobs.go
- Added Scaleway Secret Manager config provider
- Tested config lifecycle (set/ls/rm)
- Attempted to bypass Fabric auth — reverted (too many whack-a-mole changes)

### Session 2
- Confirmed Fabric auth works with DEFANG_TOKEN
- Deep-dived CLI BYOC client, Pulumi provider, and CD task code
- Identified 5 blockers (see above)
- Created this work log

### Session 3 (current - 2026-05-09)
All 5 blockers addressed:

**Blocker 1 (CD entry point) - FIXED**
- Changed CLI `byoc.go` to use `/app/cd` instead of `node lib/index.js`
- Added `DEFANG_PULUMI_DIR` debug support for local CD testing

**Blocker 2 (CD provider detection) - FIXED**
- Added `SCW_DEFAULT_PROJECT_ID` and `SCW_DEFAULT_REGION` to `cd/envs.go`
- Added Scaleway detection in `cd/main.go:stackConfig()` BEFORE AWS check
  (Scaleway sets `AWS_REGION` for S3 compatibility, would otherwise trigger AWS provider)
- Added `"scaleway"` to `disable-default-providers` list

**Blocker 3 (Image building) - FIXED**
- Created `provider/defangscaleway/build.go` - Build custom resource using Kaniko
  via Scaleway Serverless Jobs API
- Created `provider/defangscaleway/scaleway/image.go` - `GetServiceImage()` and
  `buildServiceImage()` following AWS/GCP/Azure patterns
- Updated `provider/defangscaleway/project.go` to wire up build support
- Updated `provider/defangscaleway/provider.go` to register Build resource
- Approach: Kaniko debug image (with shell) runs in a temporary Serverless Job.
  Docker config for Scaleway Container Registry auth (nologin + SCW_SECRET_KEY)
  is written via shell command before running the executor.
  S3-compatible build context accessed via AWS SDK env vars with custom endpoint.

**Blocker 4 (POSTGRES_USER reserved) - FIXED**
- Changed `scalewayPostgresUsername()` to auto-remap "postgres" → "defang" with
  a Pulumi warning instead of erroring
- Same for `scalewayPostgresDBName()` ("postgres" DB → "defang")

**Blocker 5 (POSTGRES_HOST injection) - FIXED**
- Added `ManagedHosts` map to `SharedInfra` struct
- `createPostgres()` stores the actual Postgres host in `ManagedHosts[serviceName]`
- `containerEnvironment()` replaces env values matching managed service names
  with actual hostnames (e.g., `POSTGRES_HOST=database` → actual Scaleway IP/hostname)
- Also auto-remaps `POSTGRES_USER=postgres` and `POSTGRES_DB=postgres` to `defang`
  in container env when managed Postgres services exist
- Follows Azure's `serviceHosts` pattern but simpler (no postgres:// URL rewriting needed)

**Test changes:**
- Updated `TestBuildProjectRejectsBuildOnlyServices` → `TestBuildProjectCreatesBuildResourceForBuildOnlyServices`
- Added mock for `defang-scaleway:index:Build` resource type

### Session 4 (2026-05-09)

**Blocker 6 (S3 context URL format for Kaniko) - FIXED**
- Kaniko expects `s3://bucket/key` format for S3 build contexts
- The CLI's `uploadArchive()` returns HTTPS presigned URLs (with query params stripped)
- Scaleway S3 path-style URLs look like `https://s3.fr-par.scw.cloud/bucket/key`
- Added `convertScalewayS3URL()` in `cli/compose/context.go` to convert to `s3://bucket/key`
- Similar to the existing GCS conversion (`https://storage.googleapis.com/` → `gs://`)
- Added `TestConvertScalewayS3URL` test with 5 cases

**Blocker 7 (S3_ENDPOINT missing in CD environment) - FIXED**
- The CD task's `fetchS3()` in `cd/fetch.go` uses `S3_ENDPOINT` env var for custom endpoints
- Scaleway BYOC client wasn't passing this env var to the CD task
- Added `"S3_ENDPOINT": scaleway.S3Endpoint(b.region)` to the CD env map in `byoc.go`
- Note: `S3Endpoint()` already returns full URL with `https://` prefix

**Blocker 8 (Config/secrets not found - POSTGRES_PASSWORD) - FIXED**
- Root cause: Scaleway provider used `PulumiConfigProvider` which reads from Pulumi stack config
- Secrets are in Scaleway Secret Manager, not in Pulumi config
- AWS/GCP/Azure each have cloud-native ConfigProviders; Scaleway was missing one
- Created `provider/defangscaleway/scaleway/parameters.go` with Scaleway ConfigProvider
- Uses direct REST API calls to Scaleway Secret Manager (not Pulumi SDK invokes)
  because `disable-default-providers` blocks the Scaleway provider for data source invokes too
- Bulk-fetches all secrets with prefix `Defang_<project>_<stack>_<KEY>`, caches results
- Base64-decodes secret data from API response
- Updated `project.go` to use `NewConfigProvider(projectName)` instead of `PulumiConfigProvider`

**Blocker 9 (Default provider for 'scaleway' disabled) - FIXED**
- CD task's `projectConfig()` disables all default providers including `scaleway`
- All other providers (AWS/GCP/Azure) create explicit providers in their `deploy*` functions
- Added explicit `scaleway.NewProvider` in `cd/program/scaleway.go`
- Passes via `pulumi.Providers(scwProvider)` to `NewProject`
- Added `github.com/pulumiverse/pulumi-scaleway` dependency to `cd/go.mod`

**Blocker 10 (Kaniko build job shell quoting) - FIXED**
- The Scaleway Serverless Jobs API `command` field splits strings by whitespace into exec arrays
- Neither `command` (string) nor `startup_command` (array) wraps in a shell automatically

**Key findings from extensive testing (sessions 4-5):**
1. `command` (string): splits by whitespace, overrides Docker ENTRYPOINT+CMD, IS inherited by job runs
2. `startup_command` (array): sets Docker CMD only, NOT inherited by job runs
3. `args` (array): passes args to command, NOT inherited by job runs
4. Only `command` and `environment_variables` are reliably inherited by job runs
5. Shell operators (&&, >, etc.) in `command` string are NOT interpreted — they're literal exec args
6. The start endpoint returns `{"job_runs": [...]}` not a flat object

**Failed approaches (session 4):**
- `sh -c 'script'` → "unterminated quoted string" (API splits on whitespace, quotes are literal)
- `startup_command: ["sh", "-c", "script"]` → kaniko help text (appends to ENTRYPOINT, doesn't override)
- `command: "sh"` + `startup_command: ["-c", "script"]` → sh exits 0 immediately (startup_command NOT inherited)
- `command: "sh"` + `args: ["-c", "script"]` → sh exits 0 immediately (args NOT inherited)

**Solution found (session 5): `eval$IFS$KANIKO_SCRIPT` env var bootstrap**
- Store the full build script in `KANIKO_SCRIPT` environment variable (inherited by runs)
- Set `command: "sh -c eval$IFS$KANIKO_SCRIPT"`
- API whitespace-splits to exec array: `["sh", "-c", "eval$IFS$KANIKO_SCRIPT"]`
- sh receives script text `eval$IFS$KANIKO_SCRIPT`:
  - `$IFS` expands to space/tab/newline (no literal whitespace in the token for API splitting)
  - `$KANIKO_SCRIPT` expands to the full build script content
  - `eval` re-parses the expansion as shell code (enabling &&, >, pipes, etc.)
- **Verified working** with Scaleway Serverless Jobs experiments:
  - Experiment 1: Simple multi-command script with mkdir, echo, cat → succeeded
  - Experiment 2: JSON env var with quotes (mimicking Docker config) → succeeded

**Also fixed:**
- `runJob()` now correctly parses `{"job_runs": [...]}` response from start endpoint
- Removed `--compressed-caching=false` from Kaniko flags (not valid in current Kaniko version)

### Session 5 (2026-05-09)

**Blocker 11 (AWS SDK v2 credential loading in Kaniko) - FIXED**
- AWS SDK v2 in Kaniko fell through to IMDS despite `AWS_ACCESS_KEY_ID`/`AWS_SECRET_ACCESS_KEY` being present
- Error: "no EC2 IMDS role found" — SDK reaches IMDS as last resort
- Fix: Added `AWS_EC2_METADATA_DISABLED=true` to Kaniko environment to prevent IMDS fallback
- Verified with Scaleway experiment: job succeeded with IMDS disabled

**Blocker 12 (Kaniko chown fails in sandbox - CAP_CHOWN) - FIXED**
- Error: `chown /etc/gshadow: operation not permitted` during base image rootfs extraction
- `--force` flag only bypasses container detection, NOT chown during layer extraction
- Fix: Built a **patched Kaniko binary** that ignores `EPERM` on `os.Chown()` in:
  - `setFilePermissions()` — file layer extraction
  - `mkdirAndChown()` — directory creation during extraction
  - `resetFileOwnership()` — ownership reset
  - `CopyOwnership()` — cross-stage file copying
- Patched Kaniko image pushed to `rg.fr-par.scw.cloud/defang-cd/kaniko-executor:patched`
- Used `crane append` to overlay patched binary on `gcr.io/kaniko-project/executor:debug`

**Blocker 13 (Kaniko local_storage exceeded) - FIXED**
- Default Scaleway Serverless Jobs local storage is 1 GB — not enough for node builds
- API field is `local_storage_capacity` (not `local_storage_limit`)
- Set to 10000 MB (10 GB) in job definition

**Blocker 14 (Kaniko staging directory permissions) - FIXED**
- Error: `mkdir /kaniko/0/app: permission denied` — multi-stage file saving failed
- Root cause: Kaniko uses `os.MkdirAll(dstDir, 0644)` for staging dirs — mode 0644 lacks execute bit
- Fix 1: Set `KANIKO_DIR=/workspace` to use writable directory
- Fix 2: Patched `MkdirAll` to use `0755` instead of `0644` in build.go

**Blocker 15 (apt-get setgroups/seteuid in sandbox) - FIXED**
- Error: `setgroups 65534 failed - Operation not permitted` in apt-get
- Sandbox lacks CAP_SETGID/CAP_SETUID — apt-get can't drop privileges
- Fix: Patched Kaniko to inject sandbox fixups after each rootfs extraction:
  - `APT::Sandbox::User "root"` in `/etc/apt/apt.conf.d/99nosandbox`
  - Stub `adduser`/`addgroup`/`groupadd`/`useradd` with `#!/bin/sh\nexit 0`

**Blocker 16 (Health check missing required fields) - FIXED**
- Error: `health_check.0.failure_threshold is required`
- Scaleway requires `failure_threshold` and `interval` in health check config
- Fix: Added defaults (failure_threshold=3, interval=10s) when not specified

**DEPLOYMENT SUCCEEDED!**
- Build: 117s (2nd run with cache), container created: 38s
- Endpoint: `https://nextjspostgres8a2ca06a-app.functions.fnc.fr-par.scw.cloud`
- Database: `172.16.0.2:5432` (Scaleway managed PostgreSQL)
- App responds with HTTP 404 (may need time to connect to DB)

**Experiments run (session 5):**
11. Kaniko S3 with AWS_EC2_METADATA_DISABLED=true → succeeded (job exited 0)
12. Scaleway API `local_storage_capacity` field testing → confirmed field name

---

## POSTGRES_SSL analysis:
- The sample has `POSTGRES_SSL: null` (config value resolved at deploy time)
- Scaleway managed Postgres requires SSL (`sslmode=require` in connection URL)
- Users need to run `defang config set POSTGRES_SSL true` before deploying
- No code change needed — handled by existing config mechanism

---

## Research Notes

### CD Task Architecture
- **Old CD** (defang-mvp/pulumi/cd/): Node.js, AWS-only, uses `node lib/index.js`
- **New CD** (pulumi-defang/cd/): Go binary, multi-cloud (AWS/GCP/Azure/Scaleway), uses `/app/cd`
- The Scaleway CLI BYOC client still references the old CD entry point
- The new Go CD already has `deployScaleway()` in `cd/program/scaleway.go`

### Scaleway Pulumi Provider Capabilities
- Serverless Containers (deploy containers)
- Managed PostgreSQL (x-defang-postgres)
- Managed Redis (x-defang-redis)
- Private Networks (VPC, egress-only for containers)
- Container Registry (image storage, no build service)
- Secret Manager (config/secrets)
- Cockpit/Loki (logging)
- DNS zones (domain delegation)

### Image Building
- AWS: CodeBuild (downloads context from S3, builds, pushes to ECR)
- GCP: Cloud Build (similar flow)
- Azure: ACR Tasks (Azure Container Registry build tasks)
- Scaleway: No native build service — **solved via Kaniko in Scaleway Serverless Jobs**
  - Patched Kaniko image for gVisor sandbox (chown, setgroups, apt sandbox)
  - Source context from Scaleway Object Storage (S3-compatible)
  - Built images pushed to Scaleway Container Registry
  - See `provider/defangscaleway/build.go` in pulumi-defang repo

### Scaleway Serverless Jobs API Behavior
- `command` (string): splits by whitespace into exec array, overrides ENTRYPOINT+CMD, **inherited by job runs**
- `startup_command` (array): overrides CMD only, appended to ENTRYPOINT, **NOT inherited by job runs**
- `args` (array): passes args to command, **NOT inherited by job runs**
- `environment_variables` (map): **inherited by job runs**
- Start endpoint accepts `command` override (optional) and `environment_variables` override
- Start endpoint returns `{"job_runs": [...]}` (array), NOT a flat object
- To run shell scripts: use `command: "sh -c eval$IFS$SCRIPT_ENV_VAR"` with script in env var
- Error messages truncated to 2048 chars (tail of stderr)
- Job definitions must be explicitly deleted (no auto-cleanup)
- Official Go SDK (`scaleway-sdk-go`) has NO `args`/`startup_command` fields — they may be undocumented

### Authentication
- CLI uses `DEFANG_ACCESS_TOKEN` env var (NOT `DEFANG_TOKEN`)
- Token stored via `TokenStore.Save()` to `~/.local/state/defang/`
- Pulumi uses `PULUMI_BACKEND_URL` with S3-compatible Scaleway storage
- Pulumi locks stored in S3 — must cancel stale locks manually after killed deployments

### Scaleway Sandbox Limitations (Serverless Jobs/Containers)
- No `CAP_CHOWN` — cannot change file ownership
- No `CAP_SETGID`/`CAP_SETUID` — cannot switch users/groups
- No `setgroups` syscall — apt-get privilege dropping fails
- No `setegid`/`seteuid` — process credential changes fail
- `local_storage_capacity` defaults to 1 GB, max tested: 10 GB
- Job definitions accept but silently ignore unknown fields

### Patched Kaniko Image
- Source: `gcr.io/kaniko-project/executor:debug` (v1.24.0)
- Location: `rg.<region>.scw.cloud/defang-cd/kaniko-executor:patched`
- Patches applied:
  1. Ignore `EPERM` on all `os.Chown()` calls in `pkg/util/fs_util.go`
  2. Fix `os.MkdirAll` from `0644` to `0755` in `pkg/executor/build.go`
  3. After rootfs extraction, inject apt sandbox config and **functional** user mgmt stubs
- Functional stubs write actual `/etc/passwd` and `/etc/group` entries (not just `exit 0`)
- Built with `crane append` — adds a single layer replacing `/kaniko/executor`
- Build from source: clone `kaniko v1.24.0`, apply patches in `sandbox_fixups.go` + `fs_util.go` + `build.go`

### Next Steps
1. Verify app connectivity to managed PostgreSQL
2. Run unit tests for Scaleway provider changes
3. Add unit tests for ConfigProvider
4. Clean up `--verbosity=debug` from Kaniko flags
5. Consider making patched Kaniko image build automated (CI/CD)

---

## Session 7: Fix adduser/Kaniko stubs and end-to-end deployment (2026-05-09/10)

**Blocker 17 (Container fails: USER appuser not in /etc/passwd) - FIXED**
- Error: `unable to find user appuser: no matching entries in passwd file`
- Root cause: Patched Kaniko's adduser/addgroup stubs were `#!/bin/sh\nexit 0` — they didn't
  actually create entries in `/etc/passwd` or `/etc/group`
- The `USER appuser` Dockerfile directive was preserved in the image config but the user
  didn't exist, so the container runtime failed on startup
- Fix: Rebuilt patched Kaniko with **functional stubs** in `pkg/executor/sandbox_fixups.go`:
  - `adduser`/`useradd` stubs parse flags and append to `/etc/passwd`
  - `addgroup`/`groupadd` stubs parse flags and append to `/etc/group`
  - Stubs handle `--system`, `--ingroup`, `--uid`, `--gid`, `--shell`, `--home` flags

**Blocker 18 (addgroup symlink overwrites adduser) - FIXED**
- In Debian-based images, `/usr/sbin/addgroup` is a **symlink** to `/usr/sbin/adduser`
- When writing the addgroup stub via `os.WriteFile`, it followed the symlink and
  overwrote the adduser script with the addgroup content
- Result: `adduser` became the addgroup stub, which only creates groups, not users
- Fix: Call `os.Remove()` on all stub paths before writing to break symlinks

**Blocker 19 (Kaniko cache reuses old broken layers) - FIXED**
- Even after pushing new patched Kaniko, the `--cache=true` flag caused Kaniko to reuse
  cached layers from previous builds (where adduser was a no-op)
- Fix: Deleted all cache tags from Scaleway Container Registry before redeploying

**FULL CYCLE VERIFIED!**
- Config → Up → Verify → Down all working
- Container status: `ready` (not `error`)
- Endpoint: `https://nextjspostgresc1cb211c-app.functions.fnc.fr-par.scw.cloud` (HTTP 200)
- App renders "Recent IP Responses stored in Postgres database managed by Defang"
- Postgres data reads/writes confirmed (visitor entries with timestamps)
- Clean teardown: 0 container namespaces remain after destroy

**Experiments run (session 7):**
13. Local Kaniko test with functional adduser stubs → /etc/passwd populated correctly
14. Multi-stage Dockerfile test → fixups injected for all stages with RUN commands
15. Verified symlink bug: addgroup→adduser symlink caused os.WriteFile to overwrite adduser
16. Full cycle test: config set → compose up → curl endpoint → compose down → verify cleanup

**Known issue (not blocking):**
- Log tailing fails because `fr-par.logs.cockpit.scaleway.com` doesn't resolve from this environment
- The CLI exits with an error after the CD job is dispatched, but the deployment completes successfully
- This is a DNS/network issue in the devcontainer, not a Scaleway or CLI bug

---

## Session 8: LLM / Managed Inference Support (2026-05-10)

**Goal:** Add Scaleway Managed Inference (Generative APIs) support to match the LLM access
gateway pattern used by AWS (Bedrock) and GCP (Vertex AI).

### Research Findings

1. **Scaleway Generative APIs** — OpenAI-compatible at `https://api.scaleway.ai/v1`
   - Auth: `Authorization: Bearer $SCW_SECRET_KEY`
   - Supports chat completions, embeddings, streaming, structured outputs, tool calling
   - As of May 2026, rebranded from "Managed Inference" to "Generative APIs - Dedicated Deployment"

2. **LiteLLM has native Scaleway support** — model prefix `scaleway/`, env var `SCW_SECRET_KEY`
   - Example: `scaleway/llama-3.3-70b-instruct`
   - All Scaleway Generative API models supported
   - Streaming, structured outputs, tool calling all work

3. **Available models (serverless):**
   - Chat: `llama-3.3-70b-instruct`, `qwen3-235b-a22b-instruct-2507`, `mistral-small-3.2-24b-instruct-2506`, `deepseek-r1-distill-llama-70b`, `gemma-3-27b-instruct`, etc.
   - Embedding: `bge-multilingual-gemma2`, `qwen3-embedding-8b`
   - Audio: `whisper-large-v3`
   - Vision: `pixtral-12b-2409`

### Changes Made

**CLI compose fixup (`src/pkg/cli/compose/fixup.go`):**
- Added `ProviderScaleway` case to `configureAccessGateway`
- Default chat model: `llama-3.3-70b-instruct` (widely available, good quality)
- Default embedding model: `bge-multilingual-gemma2`
- Uses LiteLLM's `scaleway/` prefix — LiteLLM handles Scaleway auth via `SCW_SECRET_KEY`
- No extra env vars needed in fixup (unlike AWS `AWS_REGION` or GCP `VERTEXAI_PROJECT`) — LiteLLM resolves Scaleway endpoint from the prefix

**Tests (`src/pkg/cli/compose/fixup_test.go`):**
- Added `TestMakeAccessGatewayServiceScaleway` with 3 subtests:
  - `chat-default` → `scaleway/llama-3.3-70b-instruct`
  - `embedding-default` → `scaleway/bge-multilingual-gemma2`
  - Custom model gets `scaleway/` prefix

**Sample app (`samples/scaleway-llm-chat/`):**
- FastAPI chat app with a conversation UI
- Uses `provider: type: model` compose pattern (matches `managed-llm-provider` sample)
- LLM service uses `x-defang-llm: true` and `provider: type: model`
- `SCW_SECRET_KEY` passed to LLM service as a secret (set via `defang config set`)
- Chat UI with send/receive, loading states, error handling

### Architecture

```
User → [app (FastAPI)] → [llm (LiteLLM)] → Scaleway Generative APIs
         :8000               :4000            api.scaleway.ai/v1
```

The `provider: type: model` pattern in the compose file triggers the CLI fixup to:
1. Replace the `llm` service with a LiteLLM container
2. Resolve `chat-default` to `scaleway/llama-3.3-70b-instruct` for the Scaleway provider
3. Inject `LLM_URL`, `LLM_MODEL`, `OPENAI_API_KEY` into the `app` service
4. Put both services on a private network

### Deployment Steps (to test)

```bash
cd samples/scaleway-llm-chat
defang config set SCW_SECRET_KEY   # Scaleway API key
defang compose up                   # Deploy to Scaleway
```

### Status
- [x] CLI fixup code: DONE
- [x] Unit tests: DONE (need CI to verify)
- [x] Sample app: DONE
- [x] End-to-end deployment: DONE (see Session 9 below)

---

## Session 9: LLM Chat Deployment Success (2026-05-10)

### What was done

Successfully deployed the `scaleway-llm-chat` sample app to Scaleway using `defang compose up`.

### Key findings and fixes

1. **CD Image was empty (0 bytes)**: The `rg.fr-par.scw.cloud/defang-cd/cd:scaleway` image had no content. Rebuilt it using `crane` with:
   - Base: `alpine:3.21` (needed for CA certificates)
   - `/app/cd`: Built from `pulumi-defang` repo (`sam/httpsgithubcomdefanglabspulumi-defang-deep-dive-01kr1k` branch)
   - `/usr/local/bin/pulumi` v3.231.0 (matching Go SDK version)
   - Pre-installed Pulumi plugins: `defang-scaleway` v2.0.0-beta.5 + `scaleway` v1.48.0

2. **PULUMI_HOME permission denied**: Scaleway Serverless Jobs don't run as root. Added `PULUMI_HOME=/home/.pulumi` to the CD job environment in `byoc.go`.

3. **Host-mode ports unsupported**: The LiteLLM access gateway uses `Mode_HOST` ports, which Scaleway Serverless Containers don't support. **Workaround**: Simplified the sample to talk directly to Scaleway's Generative API (`https://api.scaleway.ai/v1/`) instead of using LiteLLM as intermediary. This avoids the host-mode port issue entirely.

4. **DEFANG_CD_IMAGE**: Must be set to `rg.fr-par.scw.cloud/defang-cd/cd:scaleway`. The ECR image (`public.ecr.aws/defang-io/cd:public-beta`) is a Node.js app without `/app/cd` binary.

### Architecture (deployed)

```
User → https://scwllmchat45eef04f-app.functions.fnc.fr-par.scw.cloud
         ↓
   FastAPI App (Scaleway Serverless Container)
         ↓ OPENAI_API_KEY=${SCW_SECRET_KEY}
   https://api.scaleway.ai/v1/ (Scaleway Generative APIs)
         ↓
   llama-3.3-70b-instruct
```

### Verified working

- Health check: `GET /health` → `{"status":"healthy"}`
- Chat: `POST /ask` with prompt → LLM response from llama-3.3-70b-instruct
- Web UI: Chat interface loads and works

### LLM approach (final)

The initial LiteLLM sidecar approach (session 8) was superseded. Scaleway LLM now uses **direct Generative API access** via CLI compose fixup — no sidecar container is deployed. The CLI strips `provider: type: model` services and injects `CHAT_URL`, `CHAT_MODEL`, `EMBEDDING_URL`, `EMBEDDING_MODEL`, and `OPENAI_API_KEY` env vars pointing to `https://api.scaleway.ai/v1/`. This avoids the host-mode port and service-to-service DNS issues that blocked the LiteLLM sidecar approach. Validated end-to-end with the `mastra-extended` sample (2026-05-10).
