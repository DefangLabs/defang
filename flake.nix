{
  outputs = { self, nixpkgs, flake-utils }: {
    overlays.default = [ (import ./nix/overlay.nix) ];
  } //
  flake-utils.lib.eachDefaultSystem
    (system:
      let
        pkgs = import nixpkgs {
          inherit system;
          overlays = self.overlays.default;
        };
      in
      {
        devShells.default = import ./shell.nix { pkgs = pkgs; };
        packages.defang-cli = pkgs.callPackage ./nix/cli.nix { };
        packages.defang-bin = pkgs.callPackage ./nix/bin.nix { };
      }
    );
}
