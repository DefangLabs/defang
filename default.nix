{
  pkgs ? import <nixpkgs> { },
}:
let
  go_1_26_10 = pkgs.go_1_26.overrideAttrs (_: {
    version = "1.26.10";
    src = pkgs.fetchurl {
      url = "https://go.dev/dl/go1.26.10.src.tar.gz";
      hash = "sha256-/WPog7ovK/ypPBZGsx6+9j4rHk0rMXyXidl2po2B828=";
    };
  });
in
pkgs.callPackage ./pkgs/defang/cli.nix { inherit go_1_26_10; }
