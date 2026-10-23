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

- CD runs on Scaleway Serverless Jobs and uses Object Storage for Pulumi state.
  Older deployments use `project.pb` for service readback.
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

## Deployment outputs

The CLI and CD share the MIT packages `pkg/deployment` and
`pkg/clouds/scaleway/contract`. The CLI does not import provider implementation
packages from `pulumi-defang`.

Scaleway service components publish a versioned `defangDeployment` output as a
JSON string containing the project, stack, deployment ID, physical resource ID,
endpoint, and runtime log selectors. The Project component publishes a summary;
CD also exports that summary as a stack output. These outputs exclude credentials
and database connection strings.

The CLI reads these outputs from the current Pulumi checkpoint. Individual
component outputs support discovery during partial deployments; resources pending
deletion or replacement are excluded. A selected deployment ID prevents mixing
attempts. Unknown contract versions require a CLI update.

Runtime logs use the published selectors, with records mapped back to Compose
service names. Container status is queried from Scaleway using the published ID;
it describes control-plane readiness, not application health. Managed service
health is reported as unknown. CD job completion remains a separate signal.

When outputs are absent, service readback falls back to `project.pb`. Runtime log
discovery requires redeploying with a CD image that publishes these outputs. CD
logs remain available before application state exists. Build logs retain the
preview implementation's existing job-prefix selector.

Config names are needed before provisioning, so both sides use the shared
`contract.ConfigName` and `contract.ConfigPrefix` helpers rather than discovering
them from application state.
