Estimate the monthly cloud cost for the current project using the Defang CLI by following these steps:

## Step 1: Validate the CLI

Run `defang --version` to confirm the CLI is installed. If it is not found, install it:

- npm: `npm install -g @defang-io/defang`
- Homebrew: `brew install DefangLabs/defang/defang`
- curl: `eval "$(curl -fsSL s.defang.io/install)"`

## Step 2: Authenticate

Run `defang whoami` to check if the user is already logged in. If not, run `defang login` and wait for authentication to complete.

## Step 3: Find or Create a compose.yaml file

Look for any compose.yaml files in the current directory or parent directories. If multiple are found, ask the user to select one. If none are found, ask the user if they want to create a new one. If yes, ask them about the services they want to deploy and generate a compose.yaml based on their answers.

## Step 4: Determine target stack

A stack specifies the cloud provider, region, and deployment mode used for the estimate.

List existing stacks:

```bash
defang stack list
```

- If one or more stacks exist, ask the user which one to base the estimate on.
- If no stacks exist, ask the user for:
  - **Provider**: `aws` or `gcp`
  - **Region**: e.g., `us-east-1` (AWS), `us-central1` (GCP)
  - **Deployment Mode**: controls cost and resiliency
    - `affordable` — lowest cost, for dev/test (default)
    - `balanced` — general production workloads
    - `high_availability` — critical services
    - See https://docs.defang.io/docs/concepts/deployment-modes

## Step 5: Run the estimate

```bash
defang compose estimate
```

Useful flags:

- `--provider aws|gcp`: override the provider
- `--region REGION`: override the region
- `--mode affordable|balanced|high_availability`: override the deployment mode

## Step 6: Explain the results

After the estimate completes, summarize the breakdown for the user — call out the most significant cost drivers (e.g., compute, storage, data transfer) and suggest lower-cost alternatives if the estimate seems high (e.g., switching to `affordable` mode or reducing replica counts).
