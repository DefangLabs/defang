{ pkgs ? import ./nix/nixpkgs.nix {
  overlays = [ (import ./nix/overlay.nix) ];
 } }:
with pkgs;
mkShell {
  buildInputs = [
    buf
    git
    gnumake
    gnused # force Linux `sed` everywhere
    go_1_20
    grpc-tools
    nixfmt
    nodejs-18_x # for Pulumi, must match values in package.json
    protoc-gen-go
    protoc-gen-go-grpc
    pulumi-bin
  ] ++ lib.optionals stdenv.isDarwin [
    darwin.apple_sdk.frameworks.CoreServices
  ];
}
