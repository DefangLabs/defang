{ pkgs ? import <nixpkgs> {} }:
with pkgs;
mkShell {
  buildInputs = [
    buf
    git
    gnumake
    gnused # force Linux `sed` everywhere
    go_1_20
    nixfmt
    nodejs-18_x # for Pulumi, must match values in package.json
    pulumi-bin
  ] ++ lib.optionals stdenv.isDarwin [
    darwin.apple_sdk.frameworks.CoreServices
  ];
}
