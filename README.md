[![Go package](https://github.com/defang-io/defang/actions/workflows/go.yml/badge.svg?branch=main)](https://github.com/defang-io/defang/actions/workflows/go.yml)

# defang
The Defang Opinionated Platform (DOP) is a radically simpler way to build, deploy, and optimize production-ready cloud apps.
This repo includes:
* Public releases of the Defang CLI; [click here](https://github.com/defang-io/defang/releases/latest/) for the latest version
* Samples in Golang, Python, and Node.js that show how to accomplish various tasks and deploy them to the DOP using a Docker Compose file using the Defang CLI.
* Samples that show how to deploy an app using the Defang Pulumi Provider.

## Getting started
* Read our [Terms and Conditions](https://defang.io/terms-service.html)
* Download the [latest version](https://github.com/defang-io/defang/releases/latest/) of the Defang CLI. For this beta, MacOS users will have to explicitly allow running of downloaded programs in the OS security settings.
  * or use the [Nix package manager](https://nixos.org):
    * with Nix-Env: `nix-env -if https://github.com/defang-io/defang/archive/main.tar.gz`
    * with Flakes: `nix profile install github:defang-io/defang#defang-bin --refresh`
* Take a look at our [Samples folder](https://github.com/defang-io/defang/tree/main/samples) for example projects in various programming languages.
* Try the AI integration by running `defang generate`
* Start your new service with `defang compose up`

## Support
* File any issues [right here on GitHub](https://github.com/defang-io/defang/issues)

## Command completion
The Defang CLI supports command completion for Bash, Zsh, and Fish. To get the shell script for command completion, run the following command:
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

## Environment Variables
The Defang CLI recognizes the following environment variables:
* `DEFANG_ACCESS_TOKEN` - The access token to use for authentication; if not specified, uses token from `defang login`
* `DEFANG_FABRIC` - The address of the Defang Fabric to use; defaults to `fabric-prod1.defang.dev`
* `DEFANG_HIDE_HINTS` - If set to `true`, hides hints in the CLI output; defaults to `false`
* `DEFANG_HIDE_UPDATE` - If set to `true`, hides the update notification; defaults to `false`
* `NO_COLOR` - If set to any value, disables color output; by default, color output is enabled depending on the terminal
* `XDG_STATE_HOME` - The directory to use for storing state; defaults to `~/.local/state`
