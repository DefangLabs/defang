[![Go package](https://github.com/DefangLabs/defang/actions/workflows/go.yml/badge.svg?branch=main)](https://github.com/DefangLabs/defang/actions/workflows/go.yml)

# Defang

Defang is a radically simpler way for developers to develop, deploy, and debug cloud applications.

This repo includes:

- Public releases of the Defang CLI; [click here](https://github.com/DefangLabs/defang/releases/latest/) for the latest version
- Samples in Golang, Python, and Node.js that show how to accomplish various tasks and deploy them to the DOP using a Docker Compose file using the Defang CLI.
- Samples that show how to deploy an app using the [Defang Pulumi Provider](https://github.com/DefangLabs/pulumi-defang).

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
  . <(curl -Ls https://s.defang.io/install)
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
- `DEFANG_CD_BUCKET` - The S3 bucket to use for the BYOC CD pipeline; defaults to `defang-cd-bucket-…`
- `DEFANG_CD_IMAGE` - The image to use for the Continuous Deployment (CD) pipeline; defaults to `public.ecr.aws/defang-io/cd:public-beta`
- `DEFANG_DEBUG` - set this to `1` or `true` to enable debug logging
- `DEFANG_DISABLE_ANALYTICS` - If set to `true`, disables sending analytics to Defang; defaults to `false`
- `DEFANG_FABRIC` - The address of the Defang Fabric to use; defaults to `fabric-prod1.defang.dev`
- `DEFANG_HIDE_HINTS` - If set to `true`, hides hints in the CLI output; defaults to `false`
- `DEFANG_HIDE_UPDATE` - If set to `true`, hides the update notification; defaults to `false`
- `DEFANG_PREFIX` - The prefix to use for all BYOC resources; defaults to `Defang`
- `DEFANG_PROVIDER` - The name of the cloud provider to use, `auto` (default), `aws`, `digitalocean`, or `defang`
- `DEFANG_PULUMI_DIR` - Run Pulumi from this folder, instead of spawning a cloud task; requires `--debug` (BYOC only)
- `DEFANG_PULUMI_VERSION` - Override the version of the Pulumi image to use (`aws` provider only)
- `NO_COLOR` - If set to any value, disables color output; by default, color output is enabled depending on the terminal
- `TZ` - The timezone to use for log timestamps: an IANA TZ name like `UTC` or `Europe/Amsterdam`; defaults to `Local`
- `XDG_STATE_HOME` - The directory to use for storing state; defaults to `~/.local/state`

## Development
At Defang we use the [Nix package manager](https://nixos.org) for our dev environment, in conjunction with [DirEnv](https://direnv.net).

To get started quickly, install Nix and DirEnv, then create a `.envrc` file to automatically load the Defang developer environment:
```sh
echo use flake >> .envrc
direnv allow
```


