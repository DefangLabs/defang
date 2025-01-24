{
  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let pkgs = import nixpkgs { inherit system; };
      in {
        devShell = with pkgs;
          mkShell {
            buildInputs = [
              buf
              crane
              git
              gnumake
              gnupg
              gnused # force Linux `sed` everywhere
              go_1_22
              golangci-lint
              goreleaser
              less
              nixfmt
              nodejs_20 # for Pulumi, must match values in package.json
              pulumi-bin
              google-cloud-sdk
              vim
            ] ++ lib.optionals stdenv.isDarwin
              [ darwin.apple_sdk.frameworks.CoreServices ];
          };
        packages.defang-cli = pkgs.callPackage ./pkgs/defang/cli.nix { };
        packages.defang-bin = pkgs.callPackage ./pkgs/defang { };
      });
}
