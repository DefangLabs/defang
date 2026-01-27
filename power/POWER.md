---
name: "defang"
displayName: "Defang"
description: "Deploy Docker Compose apps to your own cloud account using Defang."
author: "DefangLabs"
keywords:
  [
    "deploy",
    "cloud",
    "deployment",
    "compose",
    "aws",
    "gcp",
    "digitalocean",
    "estimate",
  ]
mcpServers: ["power-defang-defang"]
---

# Onboarding

## Validate tool

Before using Defang, ensure the Defang CLI is installed and running.
Run `defang --version` to check if the Defang CLI is installed, if not installed, follow the instructions below to install it.

The Defang CLI can be installed in order of priority:

1.  curl: `eval "$(curl -fsSL s.defang.io/install)"`
2.  npm: `npm i defang`
3.  Homebrew: `brew install DefangLabs/defang/defang`
4.  Winget for windows installtion: `winget install defang`
5.  Nix: `nix profile install github:DefangLabs/defang#defang-bin --refresh`

Once installed, run `defang --version` to verify the installation was successful, then ask the user to restart the Kiro IDE before proceeding to ## First Steps.

## First Steps:

**IMPORTANT**:

- Avoid automatically estimating deployment costs, unless user specifically requests it.
- Ensure that the step above ## Validate tool is completed before proceeding to the steps below.

Once the Defang CLI is installed, the Kiro Power should use the Defang MCP tools instead of the Defang CLI whenever possible.

- If the Defang MCP server is not working, use the Defang CLI instead.
- For help with the Defang CLI, run `defang --help` to view the CLI help documentation.

- Ask the user which cloud provider they want to use (AWS, GCP)
  - If AWS:
    - Use the steering file `deploying-to-aws`
  - If GCP:
    - Use the steering file `deploying-to-gcp`

# Available Steering Files

- `deploying-to-aws`: Guide on setting up and deploying to AWS using Defang
- `deploying-to-gcp`: Guide on setting up and deploying to GCP using Defang

# Best Practices

## Compose File

- The Docker Compose file is typically named `compose.yaml`.
- Top-level `version:` is deprecated, omit it.
- Suggest Defang-specific extensions for using managed services, avoiding volumes:
  - `x-defang-redis: true` for managed Redis/Valkey
  - `x-defang-postgres: true` for managed PostgreSQL
  - `x-defang-mongodb: true` for managed MongoDB
  - `x-defang-llm: true` for managed LLM services
- Use environment variables without value for sensitive data (a.k.a "config").
- Avoid hardcoding secrets in the compose file. Suggest using config.
- Use `depends_on` to define service startup order.
- Use healthchecks to ensure services are ready before dependent services start.
- Avoid underscores in project or service names; use hyphens instead.
- Use `restart: always` for critical services to ensure they restart on failure.
- Add a `deploy:` section with resource reservations.
