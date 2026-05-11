# Scaleway BYOC Integration Notes

This package is still a preview provider path. When adding or changing
Scaleway behavior, check the surfaces below so provider support does not stop
at the deploy path.

## Cross-Surface Checklist

- Provider ID parsing, display names, protobuf mapping, and default regions.
- Stack wizard provider choices and stack-name defaults.
- Agent tool JSON schemas and tests that enumerate provider choices.
- Compose fixups for managed services, models, ports, and config references.
- CD setup, run, status, subscribe, logs, teardown, and state readback.
- Docs pages, provider tables, managed LLM tables, and sample prerequisites.

## Current Preview Constraints

- CD runs on Scaleway Serverless Jobs and uses Object Storage for Pulumi state
  and `project.pb` readback.
- CD job definitions must be scoped per stack to avoid concurrent deployments
  deleting or replacing each other's job definition.
- Scaleway managed LLM support bypasses LiteLLM and injects OpenAI-compatible
  environment variables that point at `https://api.scaleway.ai/v1/`.
- Serverless Containers do not support host-mode ports; public HTTP services
  must use ingress ports.
- Portless workers use a temporary health shim in the Pulumi provider because
  Serverless Containers require an HTTP listener.
- Managed Redis currently requires `REDIS_PASSWORD` Defang config.
- Managed PostgreSQL passwords must satisfy Scaleway's password policy.
