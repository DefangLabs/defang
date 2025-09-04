## Develop Once, Deploy Anywhere.

Take your app from Docker Compose to a secure and scalable deployment on your favorite cloud in minutes.

## Defang CLI

The Defang Command-Line Interface [(CLI)](https://docs.defang.io/docs/getting-started) is designed for developers who prefer to manage their workflows directly from the terminal. It offers full access to Defang’s capabilities, allowing you to build, test, and deploy applications efficiently to the cloud.

## Getting started

- Read our [Getting Started](https://docs.defang.io/docs/getting-started) page
- Follow the installation instructions from the [Installing](https://docs.defang.io/docs/getting-started/installing) page
- Take a look at our [Samples folder](https://github.com/DefangLabs/defang/tree/main/samples) for example projects in various programming languages.
- Try the AI integration by running `defang generate`
- Start your new service with `defang compose up`

## Support

- File any issues [right here on GitHub](https://github.com/DefangLabs/defang/issues)
- Join our [Discord community](https://s.defang.io/discord) for real-time help and discussions

## Environment Variables

The Defang CLI recognizes the following environment variables:

- `COMPOSE_PROJECT_NAME` - The name of the project to use; overrides the name in the `compose.yaml` file
- `DEFANG_ACCESS_TOKEN` - The access token to use for authentication; if not specified, uses token from `defang login`
- `DEFANG_BUILD_CONTEXT_LIMIT` - The maximum size of the build context when building container images; defaults to `100MiB`
- `DEFANG_CD_BUCKET` - The S3 bucket to use for the BYOC CD pipeline; defaults to `defang-cd-bucket-…`
- `DEFANG_CD_IMAGE` - The image to use for the Continuous Deployment (CD) pipeline; defaults to `public.ecr.aws/defang-io/cd:public-beta`
- `DEFANG_DEBUG` - set this to `1` or `true` to enable debug logging
- `DEFANG_DISABLE_ANALYTICS` - If set to `true`, disables sending analytics to Defang; defaults to `false`
- `DEFANG_EDITOR` - The editor to launch after new project generation; defaults to `code` (VS Code)
- `DEFANG_FABRIC` - The address of the Defang Fabric to use; defaults to `fabric-prod1.defang.dev`
- `DEFANG_JSON` - If set to `true`, outputs JSON instead of human-readable output; defaults to `false`
- `DEFANG_HIDE_HINTS` - If set to `true`, hides hints in the CLI output; defaults to `false`
- `DEFANG_HIDE_UPDATE` - If set to `true`, hides the update notification; defaults to `false`
- `DEFANG_ISSUER` - The OAuth2 issuer to use for authentication; defaults to `https://auth.defang.io`
- `DEFANG_MODEL_ID` - The model ID of the LLM to use for the generate/debug AI integration (Pro users only)
- `DEFANG_NO_CACHE` - If set to `true`, disables pull-through caching of container images; defaults to `false`
- `DEFANG_ORG` - The name of the organization to use; defaults to the user's GitHub name
- `DEFANG_PREFIX` - The prefix to use for all BYOC resources; defaults to `Defang`
- `DEFANG_PROVIDER` - The name of the cloud provider to use, `auto` (default), `aws`, `digitalocean`, `gcp`, or `defang`
- `DEFANG_PULUMI_BACKEND` - The Pulumi backend URL or `"pulumi-cloud"`; defaults to a self-hosted backend
- `DEFANG_PULUMI_DIR` - Run Pulumi from this folder, instead of spawning a cloud task; requires `--debug` (BYOC only)
- `DEFANG_PULUMI_VERSION` - Override the version of the Pulumi image to use (`aws` provider only)
- `DEFANG_TENANT` - The name of the tenant to use.
- `NO_COLOR` - If set to any value, disables color output; by default, color output is enabled depending on the terminal
- `PULUMI_ACCESS_TOKEN` - The Pulumi access token to use for authentication to Pulumi Cloud; see `DEFANG_PULUMI_BACKEND`
- `PULUMI_CONFIG_PASSPHRASE` - Passphrase used to generate a unique key for your stack, and configuration and encrypted state values
- `TZ` - The timezone to use for log timestamps: an IANA TZ name like `UTC` or `Europe/Amsterdam`; defaults to `Local`
- `XDG_STATE_HOME` - The directory to use for storing state; defaults to `~/.local/state`

Environment variables will be loaded from a `.defangrc` file in the current directory, if it exists. This file follows
the same format as a `.env` file: `KEY=VALUE` pairs on each line, lines starting with `#` are treated as comments and ignored.
