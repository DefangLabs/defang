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
                go_1_22
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

            # Add a shellHook to set up the pre-push Git hook
            shellHook = ''
              # Check if the .git directory exists
              if [ ! -d .git ]; then
                echo "Initializing Git repository..."
                git init
              fi

              # Ensure the .git/hooks directory exists
              mkdir -p .git/hooks

              # Install the pre-push Git hook
              if [ ! -f .git/hooks/pre-push ]; then
                ln -sf ../../scripts/git-hooks/pre-push .git/hooks/pre-push
                chmod +x .git/hooks/pre-push
                echo "Pre-push hook installed."
              fi
            '';
          };
        packages.defang-cli = pkgs.callPackage ./pkgs/defang/cli.nix { };
        packages.defang-bin = pkgs.callPackage ./pkgs/defang { };
      }
    );
}
