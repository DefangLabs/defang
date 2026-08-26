{
  # Prebuilt `defang-cli` for released tags, published by the nix-cache-push
  # workflow in DefangLabs/defang-mvp. Nix only honours these for trusted users
  # (root, or a member of `trusted-users`); everyone else builds from source
  # unless they add the same two lines to their own nix.conf — see the README.
  nixConfig = {
    extra-substituters = [
      "https://defang-public-readonly.s3.us-west-2.amazonaws.com/nix-cache"
    ];
    extra-trusted-public-keys = [
      "defang.io-1:G9XiaHDves1Jygn9Kbj4FO6BpfuH/1eevUNiDqb6Nyw="
    ];
  };

  outputs =
    {
      self,
      nixpkgs,
      flake-utils,
    }:
    flake-utils.lib.eachDefaultSystem (
      system:
      let
        pkgs = import nixpkgs {
          inherit system;
        };
      in
      {
        devShell =
          with pkgs;
          mkShell {
            buildInputs = [
              azure-cli
              actionlint
              bashInteractive # full bash with readline/completion so prompts render correctly
              crane
              gh
              git
              gnumake
              gnused # force Linux `sed` everywhere
              go_1_25 # must match GO_VERSION in Dockerfile
              golangci-lint
              google-cloud-sdk
              gopls
              goreleaser
              less
              nixfmt-rfc-style
              nodejs_24 # for Pulumi, must match values in package.json and npm-build/action.yml
              openssh
              protobuf # protoc
              protoc-gen-connect-go
              protoc-gen-go
              protolint
              pulumi
              pulumiPackages.pulumi-go
              pulumiPackages.pulumi-nodejs
              vim
            ];
            shellHook = ''
              unset DEVELOPER_DIR # to avoid suprious warning: unhandled Platform key FamilyDisplayName
              export SHELL=${bashInteractive}/bin/bash

              if [ -t 1 ]; then
                export PS1="[defang:nix] \w$ "
              fi
            '';
          };
        packages.defang-cli = pkgs.callPackage ./pkgs/defang/cli.nix { };
        packages.defang-bin = pkgs.callPackage ./pkgs/defang { };
      }
    );
}
