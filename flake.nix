{
  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem
      (system:
        let
          pkgs = import nixpkgs {
            inherit system;
          };
        in
        {
          devShells.default = import ./shell.nix { pkgs = pkgs; };
          packages.defang-cli = pkgs.callPackage ./nix/cli.nix { };
          packages.defang-bin = pkgs.callPackage ./pkgs/defang { };
        }
      );
}
