Check the current Defang project status by inspecting deployment state and logs for errors.

## Step 1: Validate the CLI

Run `defang --version` to confirm the CLI is installed. If it is not found, install it:

- npm: `npm install -g @defang-io/defang`
- Homebrew: `brew install DefangLabs/defang/defang`
- curl: `eval "$(curl -fsSL s.defang.io/install)"`

## Step 2: Authenticate

Run `defang whoami` to check if the user is already logged in. If not, run `defang login` and wait for authentication to complete.

## Step 3: Find the compose.yaml file

Look for any compose.yaml files in the current directory or parent directories. If multiple are found, ask the user to select one. Note the services defined — their names will be used to correlate status and log output.

## Step 4: Check the current stack

```bash
defang stack ls
```

Interpret the output as follows:

- **No stacks listed** — No deployment has been made yet. Stop here and inform the user: "No deployment found yet." Ask them to either run the project's deploy command (e.g., `/defang:deploy`) to create one, or verify they are in the correct project directory before continuing.
- **Stacks listed but none marked as default** — A deployment exists but no stack is active. Ask the user which stack to check, then set it as the default:
  ```bash
  defang stack default STACK_NAME
  ```
- **A stack is already active** — Note its provider, region, and deployment mode and continue to the next step.

## Step 5: Check deployment status

```bash
defang compose ps
```

For each service, note:
- Whether it is running, stopped, or in an error state
- The number of healthy vs. unhealthy replicas
- Any listed endpoints

Flag any service that is not in a healthy running state for closer investigation.

## Step 6: Fetch recent logs and look for errors

Fetch the last few minutes of logs across all services:

```bash
defang logs --since 15m
```

To narrow down to a specific service showing problems:

```bash
defang logs SERVICE_NAME --since 15m
```

Scan the output for:
- Stack traces or panics
- HTTP 5xx responses
- `error`, `fatal`, `exception`, `crash`, or `OOM` keywords
- Repeated restart or exit messages
- Connection failures (database, external APIs, etc.)

If the initial window misses relevant context, expand it:

```bash
defang logs SERVICE_NAME --since 1h
```

## Step 7: Summarize and recommend

After gathering the above information, provide a concise diagnosis:

1. **Status summary** — which services are healthy, degraded, or down
2. **Root cause** — the most likely error based on log evidence, with the relevant log lines quoted
3. **Next steps** — specific, actionable recommendations (e.g., fix a misconfigured env var, increase memory, redeploy after a code fix)

If the issue is unclear, suggest running `defang compose up` with logs streaming to observe the next deployment in real time.
