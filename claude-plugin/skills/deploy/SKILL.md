---
description: Deploy the current project to the cloud using Defang. Guides through CLI setup, authentication, compose file creation, stack selection, config management, and deployment.
---

Deploy the current project using the Defang CLI by following these steps:

## Step 1: Validate the CLI

Run `defang --version` to confirm the CLI is installed. If it is not found, install it:

- npm: `npm install -g @defang-io/defang`
- Homebrew: `brew install DefangLabs/defang/defang`
- curl: `eval "$(curl -fsSL s.defang.io/install)"`

If you installed defang for the first time, run `/reload-plugins` to activate the Defang MCP server.

## Step 2: Authenticate

Run `defang whoami` to check if the user is already logged in. If not, run `defang login` and wait for authentication to complete.

## Step 3: Find or Create a compose.yaml file

Look for any compose.yaml files in the current directory or parent directories. If multiple are found, ask the user to select one. If none are found, ask the user if they want to create a new one. If yes, ask them about the services they want to deploy and generate a compose.yaml based on their answers. You will need to know which command each service should run, any environment variables they require, whether or not they should listen on a port, and if so, which one, and so on.

## Step 4: Select or create a stack

A stack is a single deployed instance of the project in a specific cloud account and region, for example: `production` is deployed to `aws` in `us-east-1`. Additional configuration associated with a stack can be specified in the stack file (e.g., `.defang/production`). Note that stack variables are read at deployment time, and are not available at runtime in your application services.

List existing stacks:

```bash
defang stack list
```

- If one or more stacks exist, ask the user whether to use an existing one or create a new one.
- If no stacks exist, create a new one.

To create a stack interactively:

```bash
defang stack new
```

Required parameters:

- **Name**: alphanumeric, cannot start with a number (e.g., `production`)
- **Provider**: `aws`, `gcp`, or `digitalocean`
- **Region**: e.g., `us-east-1` (AWS), `us-central1` (GCP)
- **Deployment Mode**: controls cost and resiliency
  - `affordable` — lowest cost, for dev/test (default)
  - `balanced` — general production workloads
  - `high_availability` — critical services
  - See https://docs.defang.io/docs/concepts/deployment-modes

After creating a stack, set it as the default:

```bash
defang stack default STACK_NAME
```

## Step 5: Set required configs

Check the `compose.yaml` for environment variables listed without a value — these must be set as configs before deployment.

List configs already set:

```bash
defang config ls
```

For each unset config, ask the user whether to provide a specific value or generate a random one. Do not assume a default. Then set it:

```bash
# specific value
defang config set KEY=value

# random value
defang config set --random KEY

# prompted securely (interactive)
defang config set KEY
```

## Step 6: Deploy

```bash
defang compose up
```

Useful flags:

- `-d` / `--detach`: return immediately without streaming logs
- `--force`: force a fresh image build even if nothing changed
- `--mode affordable|balanced|high_availability`: override the stack's deployment mode

The CLI will stream logs automatically. If the deployment fails, Defang may offer an AI-assisted debug prompt — allow it to run before investigating manually.

## Step 7: Verify

Once deployment completes, check service status and endpoints:

```bash
defang compose ps
```

To tail logs after the fact:

```bash
defang logs --follow
defang logs SERVICE_NAME --follow   # filter to one service
```
