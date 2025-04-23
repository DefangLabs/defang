[![Go package](https://github.com/DefangLabs/defang/actions/workflows/go.yml/badge.svg?branch=main)](https://github.com/DefangLabs/defang/actions/workflows/go.yml)

# ![Defang](https://raw.githubusercontent.com/DefangLabs/defang-assets/main/Logos/Element_Wordmark_Slogan/JPG/Dark_Colour_Glow.jpg)

**Develop Anything. Deploy Anywhere.**

Take your app from Docker Compose to a secure, scalable deployment on your favorite cloud in minutes — with zero config stress. Whether you prefer the command line or your favorite IDE, Defang gives you full control over how you deploy projects.

**Defang CLI**  
For power users who live in the terminal. The Defang CLI gives you full access to everything Defang offers — from local dev to live cloud deployments. Build, test, and deploy directly from your shell.

**Defang MCP Server**  
Prefer to work inside your IDE? The Defang MCP Server supports seamless deployments from tools like Cursor, VSCode, VSCode Insiders, Claude, and Windsurf. It's built for developers who want powerful cloud deployment baked directly into their editor workflow — no context switching required.

**What’s in This Repo**

- **Latest Defang CLI**  
  Grab the [latest release](https://github.com/DefangLabs/defang/releases/latest) and start deploying.

- **Built-in Support for MCP Server**  
  The **Model Context Protocol** (MCP) makes cloud deployment as easy as a single prompt. [Learn more](https://docs.defang.io/docs/concepts/mcp).

- **Sample Projects**  
  [Code samples](https://github.com/DefangLabs/samples) in **Golang**, **Python**, and **Node.js** to help you go from zero to deployed using Docker Compose and the Defang CLI.

- **Pulumi Integration**  
  Prefer infrastructure as code? Check out our [Pulumi Provider](https://github.com/DefangLabs/pulumi-defang) for deploying with Pulumi + Defang.

## Getting started

- Read our [Getting Started](https://docs.defang.io/docs/getting-started) page
- Follow the installation instructions from the [Installing](https://docs.defang.io/docs/getting-started/installing) page
- Take a look at our [Samples folder](https://github.com/DefangLabs/defang/tree/main/samples) for example projects in various programming languages.
- Try the AI integration by running `defang generate`
- Start your new service with `defang compose up`

## Installing

Install the Defang CLI from one of the following sources:

* Using the [Homebrew](https://brew.sh) package manager [DefangLabs/defang tap](https://github.com/DefangLabs/homebrew-defang):
  ```
  brew install DefangLabs/defang/defang
  ```

* Using a shell script:
  ```
  eval "$(curl -fsSL s.defang.io/install)"
  ```

* Using [Go](https://go.dev):
  ```
  go install github.com/DefangLabs/defang/src/cmd/cli@latest
  ```

* Using the [Nix package manager](https://nixos.org):
  - with Nix-Env:
    ```
    nix-env -if https://github.com/DefangLabs/defang/archive/main.tar.gz
    ```
  - or with Flakes:
    ```
    nix profile install github:DefangLabs/defang#defang-bin --refresh
    ```

* Using [winget](https://learn.microsoft.com/en-us/windows/package-manager/winget/):
  ```
  winget install defang
  ```

* Using a PowerShell script:
  ```
  iwr https://s.defang.io/defang_win_amd64.zip -OutFile defang.zip
  Expand-Archive defang.zip . -Force
  ```

* Download the [latest binary](https://github.com/DefangLabs/defang/releases/latest/) of the Defang CLI.

## Support

- File any issues [right here on GitHub](https://github.com/DefangLabs/defang/issues)

## Command completion

The Defang CLI supports command completion for Bash, Zsh, Fish, and Powershell. To get the shell script for command completion, run the following command:

```
defang completion [bash|zsh|fish|powershell]
```

If you're using Bash, you can add the following to your `~/.bashrc` file:

```
source <(defang completion bash)
```

If you're using Zsh, you can add the following to your `~/.zshrc` file:

```
source <(defang completion zsh)
```

or pipe the output to a file called `_defang` in the directory with the completions.

If you're using Fish, you can add the following to your `~/.config/fish/config.fish` file:

```
defang completion fish | source
```

If you're using Powershell, you can add the following to your `$HOME\Documents\PowerShell\Microsoft.PowerShell_profile.ps1` file:

```
Invoke-Expression -Command (defang completion powershell | Out-String)
```

## Environment Variables

The Defang CLI recognizes the following environment variables:

- `COMPOSE_PROJECT_NAME` - The name of the project to use; overrides the name in the `compose.yaml` file
- `DEFANG_ACCESS_TOKEN` - The access token to use for authentication; if not specified, uses token from `defang login`
- `DEFANG_BUILD_CONTEXT_LIMIT` - The maximum size of the build context when building container images; defaults to `100MiB`
- `DEFANG_CD_BUCKET` - The S3 bucket to use for the BYOC CD pipeline; defaults to `defang-cd-bucket-…`
- `DEFANG_CD_IMAGE` - The image to use for the Continuous Deployment (CD) pipeline; defaults to `public.ecr.aws/defang-io/cd:public-beta`
- `DEFANG_DEBUG` - set this to `1` or `true` to enable debug logging
- `DEFANG_DISABLE_ANALYTICS` - If set to `true`, disables sending analytics to Defang; defaults to `false`
- `DEFANG_FABRIC` - The address of the Defang Fabric to use; defaults to `fabric-prod1.defang.dev`
- `DEFANG_HIDE_HINTS` - If set to `true`, hides hints in the CLI output; defaults to `false`
- `DEFANG_HIDE_UPDATE` - If set to `true`, hides the update notification; defaults to `false`
- `DEFANG_MODEL_ID` - The model ID of the LLM to use for the generate/debug AI integration (Pro users only)
- `DEFANG_NO_CACHE` - If set to `true`, disables pull-through caching of container images; defaults to `false`
- `DEFANG_ORG` - The name of the organization to use; defaults to the user's GitHub name
- `DEFANG_PREFIX` - The prefix to use for all BYOC resources; defaults to `Defang`
- `DEFANG_PROVIDER` - The name of the cloud provider to use, `auto` (default), `aws`, `digitalocean`, `gcp`, or `defang`
- `DEFANG_PULUMI_BACKEND` - The Pulumi backend URL or `"pulumi-cloud"`; defaults to a self-hosted backend
- `DEFANG_PULUMI_DIR` - Run Pulumi from this folder, instead of spawning a cloud task; requires `--debug` (BYOC only)
- `DEFANG_PULUMI_VERSION` - Override the version of the Pulumi image to use (`aws` provider only)
- `NO_COLOR` - If set to any value, disables color output; by default, color output is enabled depending on the terminal
- `PULUMI_ACCESS_TOKEN` - The Pulumi access token to use for authentication to Pulumi Cloud; see `DEFANG_PULUMI_BACKEND`
- `PULUMI_CONFIG_PASSPHRASE` - Passphrase used to generate a unique key for your stack, and configuration and encrypted state values
- `TZ` - The timezone to use for log timestamps: an IANA TZ name like `UTC` or `Europe/Amsterdam`; defaults to `Local`
- `XDG_STATE_HOME` - The directory to use for storing state; defaults to `~/.local/state`

## Development
At Defang we use the [Nix package manager](https://nixos.org) for our dev environment, in conjunction with [DirEnv](https://direnv.net).

To get started quickly, install Nix and DirEnv, then create a `.envrc` file to automatically load the Defang developer environment:
```sh
echo use flake >> .envrc
direnv allow
```


