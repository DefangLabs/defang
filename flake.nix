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
              buf
              crane
              git
              gnumake
              less
              gnused # force Linux `sed` everywhere
              go_1_24 # must match GO_VERSION in Dockerfile
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
              # Install genkit-cli locally in the project if not already present
              if [ ! -f "node_modules/.bin/genkit" ]; then
                echo "Installing genkit-cli locally..."
                npm install genkit-cli
              fi

              # Add local node_modules/.bin to PATH
              export PATH="$PWD/node_modules/.bin:$PATH"
            '';
          };
        packages.defang-cli = pkgs.callPackage ./pkgs/defang/cli.nix { };
        packages.defang-bin = pkgs.callPackage ./pkgs/defang { };
      }
    );
}
