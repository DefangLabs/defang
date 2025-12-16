{
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
              bashInteractive # full bash with readline/completion so prompts render correctly
              buf
              crane
              git
              gnumake
              less
              gnused # force Linux `sed` everywhere
              go_1_24 # must match GO_VERSION in Dockerfile
              gopls
              golangci-lint
              goreleaser
              nixfmt-rfc-style
              nodejs_24 # for Pulumi, must match values in package.json
              openssh
              pulumi
              pulumiPackages.pulumi-nodejs
              google-cloud-sdk
              vim
            ];
            shellHook = ''
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
