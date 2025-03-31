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
            buildInputs =
              [
                buf
                crane
                git
                gnumake
                less
                gnused # force Linux `sed` everywhere
                go_1_23
                golangci-lint
                goreleaser
                nixfmt-rfc-style
                nodejs_20 # for Pulumi, must match values in package.json
                openssh
                pulumi-bin
                google-cloud-sdk
                vim
              ]
              ++ lib.optionals stdenv.isDarwin [
                darwin.apple_sdk.frameworks.CoreServices
              ];
          };
        packages.defang-cli = pkgs.callPackage ./pkgs/defang/cli.nix { };
        packages.defang-bin = pkgs.callPackage ./pkgs/defang { };
      }
    );
}
