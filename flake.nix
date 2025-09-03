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
              go_1_24
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
          };
        packages.defang-cli = pkgs.callPackage ./pkgs/defang/cli.nix { };
        packages.defang-bin = pkgs.callPackage ./pkgs/defang { };
      }
    );
}
