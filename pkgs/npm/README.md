# ![Defang](https://raw.githubusercontent.com/DefangLabs/defang-assets/main/Logos/Element_Wordmark_Slogan/JPG/Dark_Colour_Glow.jpg)

Develop Anything, Deploy Anywhere. Take your app from Docker Compose to a secure and scalable deployment on your favorite cloud in minutes.

- Public releases of the Defang CLI; [click here](https://github.com/DefangLabs/defang/releases/latest/) for the latest version
- Sample projects in Golang, Python, Node.js, and many more languages and frameworks, that show you how to accomplish various tasks and deploy them to the DOP using a Docker Compose file and the Defang CLI.
- Samples that show how to deploy an app using the [Defang Pulumi Provider](https://github.com/DefangLabs/pulumi-defang).

## Getting started

- Read our [Getting Started](https://docs.defang.io/docs/getting-started) page
- Follow the installation instructions from the [Installing](https://docs.defang.io/docs/getting-started/installing) page
- Take a look at our [Samples folder](https://github.com/DefangLabs/defang/tree/main/samples) for example projects in various programming languages.
- Try the AI integration by running `defang generate`
- Start your new service with `defang compose up`

## Support

- File any issues [right here on GitHub](https://github.com/DefangLabs/defang/issues)

## Environment Variables

The Defang CLI recognizes the following environment variables:

- `COMPOSE_PROJECT_NAME` - The name of the project to use; overrides the name in the `compose.yaml` file
- `DEFANG_ACCESS_TOKEN` - The access token to use for authentication; if not specified, uses token from `defang login`
- `DEFANG_CD_BUCKET` - The S3 bucket to use for the BYOC CD pipeline; defaults to `defang-cd-bucket-…`
- `DEFANG_CD_IMAGE` - The image to use for the Continuous Deployment (CD) pipeline; defaults to `public.ecr.aws/defang-io/cd:public-beta`
- `DEFANG_DEBUG` - set this to `1` or `true` to enable debug logging
- `DEFANG_DISABLE_ANALYTICS` - If set to `true`, disables sending analytics to Defang; defaults to `false`
- `DEFANG_FABRIC` - The address of the Defang Fabric to use; defaults to `fabric-prod1.defang.dev`
- `DEFANG_HIDE_HINTS` - If set to `true`, hides hints in the CLI output; defaults to `false`
- `DEFANG_HIDE_UPDATE` - If set to `true`, hides the update notification; defaults to `false`
- `DEFANG_PROVIDER` - The name of the cloud provider to use, `auto` (default), `aws`, `digitalocean`, or `defang`
- `NO_COLOR` - If set to any value, disables color output; by default, color output is enabled depending on the terminal
- `TZ` - The timezone to use for log timestamps: an IANA TZ name like `UTC` or `Europe/Amsterdam`; defaults to `Local`
- `XDG_STATE_HOME` - The directory to use for storing state; defaults to `~/.local/state`

---

Made by [Defang Labs](https://defang.io)