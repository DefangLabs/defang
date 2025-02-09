{
  pkgs ? import <nixpkgs> { },
}:
pkgs.callPackage ./pkgs/defang/cli.nix { }
