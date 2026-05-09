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

### Session 4 (current - 2026-05-09)

**Blocker 6 (S3 context URL format for Kaniko) - FIXED**
- Kaniko expects `s3://bucket/key` format for S3 build contexts
- The CLI's `uploadArchive()` returns HTTPS presigned URLs (with query params stripped)
- Scaleway S3 path-style URLs look like `https://s3.fr-par.scw.cloud/bucket/key`
- Added `convertScalewayS3URL()` in `cli/compose/context.go` to convert to `s3://bucket/key`
- Similar to the existing GCS conversion (`https://storage.googleapis.com/` → `gs://`)
- Added `TestConvertScalewayS3URL` test with 5 cases

**POSTGRES_SSL analysis:**
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

### Image Building Gap
- AWS: CodeBuild (downloads context from S3, builds, pushes to ECR)
- GCP: Cloud Build (similar flow)
- Azure: ACR Tasks (Azure Container Registry build tasks)
- Scaleway: **No native build service** — needs alternative approach
- Options: Kaniko in CD task, or add Docker-in-Docker capability
