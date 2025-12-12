---
name: "defang"
displayName: "Defang with local CLI"
description: "Deploy Docker Compose apps to your own cloud account using Defang CLI."
keywords:
  ["deploy", "cloud", "deployment", "compose", "aws", "gcp", "digitalocean"]
---

# Onboarding

## Validate tools work

Before using Defang Local MCP, ensure the following are installed and running:

- **Defang CLI**: Install via npm, Homebrew, Winget, or `eval "$(curl -fsSL s.defang.io/install)"`
  - Verify with: `defang --version`

# Best Practices

## Compose File

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
