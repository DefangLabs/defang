Prepare a project for deployment with Defang by creating or adapting a Docker Compose file (and Dockerfiles if needed). Analyze the project, identify what needs to change, and ensure everything follows Defang best practices.

## Step 1: Understand the project

Look for `compose.yaml`, `compose.yml`, `docker-compose.yaml`, or `docker-compose.yml` in the current directory.

**If a compose file exists:** Read it thoroughly and continue to Step 2 to adapt it.

**If no compose file exists:** Analyze the project to create one:

1. Examine the project structure — look for `package.json`, `go.mod`, `requirements.txt`, `Cargo.toml`, `Gemfile`, `pom.xml`, or other language markers
2. Identify the services the project needs (web server, database, cache, worker, etc.)
3. Check for an existing Dockerfile. If one exists, reference it in `build.context` and `build.dockerfile`
4. If no Dockerfile exists, either:
   - **Create a Dockerfile** appropriate for the language/framework detected, OR
   - **Use Railpack** by setting `build.context` without a `build.dockerfile` — Defang auto-detects the language and builds without a Dockerfile. Railpack supports: Node.js, Python, Go, PHP, Java, Ruby, Rust, Elixir, and static sites
5. Create a `compose.yaml` with the identified services, applying all the rules in the steps below

When creating a Dockerfile, follow standard best practices:
- Use a specific base image tag (e.g., `node:22-slim`, not `node:latest`)
- Use multi-stage builds to keep the final image small
- Copy dependency files first, install, then copy source (for layer caching)
- Set a non-root `USER` if possible
- Expose the port the app listens on
- Set an appropriate `CMD`

When creating a compose file from scratch, use `compose.yaml` as the filename (not `docker-compose.yaml`).

## Step 2: Remove unsupported directives

The following compose directives cause deployment errors and must be removed or replaced:

### Service-level (hard errors)
- `hostname` — replace with `domainname` if a custom domain is needed
- `entrypoint` — move the logic into the Dockerfile `ENTRYPOINT` or use `command` instead
- `dns` / `dns_search` — remove; Defang manages DNS
- `devices` / `device_cgroup_rules` — remove; use `deploy.resources.reservations.devices` for GPU
- `group_add` — remove

### Build-level (hard errors)
- `build.ssh` — remove; bake SSH keys into a build stage or use multi-stage builds
- `build.extra_hosts` — remove
- `build.network` — remove
- `build.secrets` — remove; use build args for non-sensitive values
- `build.tags` — remove
- `build.platforms` — remove; Defang builds for the target platform
- `build.privileged` — remove
- `build.dockerfile_inline` — move inline content to a Dockerfile
- `build.additional_contexts` — remove; restructure the build context

### Deploy-level (hard errors)
- `deploy.mode` — only `replicated` is supported (the default); remove if set to anything else
- `deploy.update_config` — remove
- `deploy.rollback_config` — remove
- `deploy.restart_policy` — remove; use the service-level `restart` field instead
- `deploy.endpoint_mode` — remove

### Top-level
- `secrets` — Defang uses its own config system instead (see Step 5)

## Step 3: Address volumes

Volumes are **not supported** by Defang. For each volume found:

1. **Database/cache volumes** — Replace the entire service with a managed service:
   - Postgres: add `x-defang-postgres: true` to the service, use a `postgres` image, set `ports: [{mode: host, target: 5432}]`, and declare `POSTGRES_PASSWORD=${POSTGRES_PASSWORD}` (so it's set via `defang config set`)
   - Redis: add `x-defang-redis: true`, use a `redis` or `valkey` image, set `ports: [{mode: host, target: 6379}]`
   - MongoDB: add `x-defang-mongodb: true`, use a `mongo` image, set `ports: [{mode: host, target: 27017}]`

2. **Application data volumes** — Warn the user that data will not persist across restarts. Recommend using an external managed service (e.g., S3, managed database).

3. **Bind-mount volumes for local development** (e.g., `.:/app`) — Remove them from the main compose file. Suggest creating a `compose.local.yaml` that uses `extends` to inherit the service and adds volumes for local use only:
   ```yaml
   # compose.local.yaml — local development only, not deployed
   services:
     app:
       extends:
         file: compose.yaml
         service: app
       volumes:
         - .:/app
   ```

Remove the top-level `volumes:` section if it becomes empty.

## Step 4: Fix port configuration

Every port must have an explicit `mode`. Review each service:

### Public-facing services (web apps, APIs serving end users)
```yaml
ports:
  - mode: ingress
    target: 8080
```
- Do NOT set `published` with `mode: ingress` — Defang manages the published port via its load balancer
- Ingress ports **require a healthcheck** (see Step 6)
- UDP is not supported with `mode: ingress`

### Internal services (databases, caches, inter-service APIs)
```yaml
ports:
  - mode: host
    target: 5432
```
- Use `mode: host` for any service that should only be reachable by other services in the project
- Databases and caches should always be `mode: host`

### Converting shorthand ports
Replace shorthand port notation with the explicit long form:
- `"8080:8080"` → `{mode: ingress, target: 8080}` (if public) or `{mode: host, target: 8080}` (if internal)
- `"5432:5432"` → `{mode: host, target: 5432}` (databases are always internal)

Remove `host_ip` if present — it is not supported.

## Step 5: Fix environment variables and secrets

### Sensitive values (passwords, API keys, tokens)
Declare them using `${VAR}` interpolation so they are set securely via `defang config set`:
```yaml
environment:
  - DATABASE_PASSWORD=${DATABASE_PASSWORD}  # Set with: defang config set DATABASE_PASSWORD
  - API_KEY=${API_KEY}                      # Set with: defang config set API_KEY
```
This explicit `${VAR}` syntax makes it clear to developers that the value comes from Defang config. Do NOT hardcode sensitive values in the compose file. Do NOT use `.env` files for secrets that should be kept out of source control.

When generating or modifying compose files, always add a YAML comment beside each config-managed variable showing the `defang config set` command needed. If the project uses multiple stacks, include `--stack <name>` in the comment.

### Service discovery references
When one service needs to connect to another, reference it by compose service name:
```yaml
environment:
  - DATABASE_URL=postgres://user:${DATABASE_PASSWORD}@db:5432/mydb
  - REDIS_URL=redis://cache:6379/0
  - API_URL=http://backend:3000
```
Here `db`, `cache`, and `backend` are compose service names. Defang automatically resolves these to the correct internal DNS. This only works for services that have `mode: host` ports.

### Variable interpolation
- `${VAR}` in **environment variables** is supported — Defang resolves it from `defang config` at deploy time
- `${VAR:-default}` is **NOT supported** at deploy time — replace with a plain default value or remove the default and set via `defang config`

### Shell environment pass-through
Defang does **not** pass the host shell's environment variables into containers (except `COMPOSE_*` variables for compose loading). If the compose file relies on shell variables being present, convert them to explicit values or `defang config` entries.

### Build args
**Build args are NOT resolved from `defang config`.** They are evaluated at build time, not deploy time, so `${VAR}` references will not be resolved from config. Any build arg without an explicit literal value will be silently dropped during the build. Always provide concrete values:
```yaml
build:
  context: .
  args:
    API_URL: https://api.example.com
```
For frontend builds that need a backend URL baked in, the build arg value should be the expected Defang endpoint URL. Ask the user what URL to use if it references a co-deployed service.

## Step 6: Add healthchecks

Every service with `mode: ingress` ports **must** have a healthcheck defined in the compose file (Dockerfile healthchecks are not used). The healthcheck must target `localhost`:

```yaml
healthcheck:
  test: ["CMD", "curl", "-f", "http://localhost:8080/"]
  interval: 30s
  timeout: 10s
  retries: 3
```

Choose the right tool based on what's available in the image:
- **curl**: `["CMD", "curl", "-f", "http://localhost:PORT/health"]`
- **wget** (Alpine): `["CMD", "wget", "--spider", "http://localhost:PORT/"]`
- **python3**: `["CMD", "python3", "-c", "import urllib.request; urllib.request.urlopen('http://localhost:PORT/')"]`

For databases/caches with `mode: host` ports, use native health tools:
- Postgres: `["CMD-SHELL", "pg_isready -U postgres"]`
- Redis: `["CMD", "redis-cli", "ping"]`

Rules:
- `timeout` must be less than `interval`
- Both must be whole seconds
- Do not use `NONE` healthcheck on services with ingress ports

## Step 7: Add resource reservations

Every compute service should specify memory reservations:

```yaml
deploy:
  resources:
    reservations:
      cpus: "0.5"
      memory: 512M
```

If only `limits` are set, move them to `reservations` (Defang uses reservations, not limits, for scheduling). If neither is set, add a reasonable default based on the service type:
- Simple web apps / APIs: `256M`–`512M`
- Backend services with processing: `512M`–`1024M`
- Databases (when not using managed): `512M`–`2048M`
- ML / AI workloads: `2048M`–`8192M`

For GPU workloads, add device reservations:
```yaml
deploy:
  resources:
    reservations:
      cpus: "2.0"
      memory: 8192M
      devices:
        - capabilities: [gpu]
          count: 1
```

## Step 8: Review Defang extensions

Check if the project would benefit from Defang-specific extensions:

| Extension | When to use |
|-----------|-------------|
| `x-defang-postgres: true` | Service uses a `postgres` or `pgvector` image |
| `x-defang-redis: true` | Service uses a `redis` or `valkey` image |
| `x-defang-mongodb: true` | Service uses a `mongo` image |
| `x-defang-llm: true` | Service is an LLM gateway (OpenAI-compatible API proxy) |
| `x-defang-static-files: /path` | Service serves static files from a directory (CDN hosting) |
| `x-defang-autoscaling: true` | Service should auto-scale based on CPU (BYOC only, Pro plan+) |

Managed database services (`x-defang-postgres`, `x-defang-redis`, `x-defang-mongodb`) provision cloud-native infrastructure (AWS RDS, ElastiCache, etc.) instead of running the database in a container. These are not available in the Playground — they require BYOC.

For managed Postgres in BYOC, the connection string must include `?sslmode=require`.

## Step 9: Check build configuration and Dockerfiles

For each service with a `build` section:

1. **Verify the Dockerfile exists** at the path specified by `build.context` + `build.dockerfile`. If no `build.dockerfile` is specified, look for `Dockerfile` in the build context directory.

2. **If no Dockerfile exists**, decide:
   - **Create one** if the project has specific build requirements, complex dependencies, or needs a multi-stage build. Follow the Dockerfile guidelines from Step 1.
   - **Use Railpack** (no Dockerfile needed) if the project is a standard app in a supported language. Railpack auto-detects: Node.js, Python, Go, PHP, Java, Ruby, Rust, Elixir, and static sites. Just set `build.context` without a `build.dockerfile`.

3. **If a Dockerfile exists**, validate it:
   - Must contain at least one `FROM` instruction
   - No instructions before `FROM` (except `ARG`)
   - Check for deprecated `MAINTAINER` (use `LABEL maintainer=` instead)
   - Ensure the exposed port matches the `ports.target` in the compose file

4. **Path rules:**
   - `build.dockerfile` must be a **relative** path (no absolute paths, no `../` prefix)
   - The Dockerfile must be inside the build context directory

5. **Add a `.dockerignore`** if one doesn't exist — at minimum exclude `node_modules`, `.git`, `__pycache__`, and build artifacts to keep the build context small (warn threshold: 100 files or 10 MiB)

6. **Build resources:** `build.shm_size` controls build memory/disk; defaults vary by provider (AWS/GCP: 16G, DO: 8G) — increase if builds fail with OOM

## Step 10: Validate the result

After making all changes, review the final compose file and present a summary to the user:

1. **Changes made** — list each modification with the reason
2. **Warnings** — things that work but may need attention:
   - Stateful images without managed extensions (data loss on restart)
   - Services without memory reservations (using provider defaults)
   - `depends_on` with managed services (does not wait for provisioning)
3. **Config values needed** — list all environment variables using `${VAR}` interpolation that must be set via `defang config set` before deployment
4. **Incompatibilities removed** — summarize what was removed and any alternatives suggested

If the project uses GitHub Actions for deployment, remind the user:
- Use `DefangLabs/defang-github-action@v1`
- Pass secrets via `config-env-vars`, not hardcoded in the workflow
- Never include `AWS_PROFILE` in `.defang/<stack>` files — it breaks CI
- OIDC auth (`id-token: write` + `AWS_ROLE_ARN`) requires the user to first connect their cloud account via Portal's OIDC configuration or by running `defang cd setup`. Without this, use static cloud credentials (e.g., `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY`) as GitHub Actions secrets instead

```yaml
# Example GitHub Actions step
- uses: DefangLabs/defang-github-action@v1
  with:
    provider: aws
    config-env-vars: |
      DATABASE_PASSWORD
      API_KEY
  env:
    AWS_REGION: us-west-2
    AWS_ROLE_ARN: arn:aws:iam::ACCOUNT_ID:role/defang-cd-CIRole
    DATABASE_PASSWORD: ${{ secrets.DATABASE_PASSWORD }}
    API_KEY: ${{ secrets.API_KEY }}
```
