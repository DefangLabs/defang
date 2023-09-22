{ pkgs ? import ./nix/nixpkgs.nix {
  overlays = [ (import ./nix/overlay.nix) ];
 } }:
with pkgs;
mkShell {
  buildInputs = [
    awscli2
    buf
    crane
    dive # for debugging Docker images
    # etcd_3_5
    # faas-cli
    # fission
    # func # knative functions
    git
    gnumake
    gnused # force Linux `sed` everywhere
    go_1_20
    grafana-loki # for logcli
    grpcurl # TEST
    grpc-client-cli # TEST
    grpc-tools
    # hey
    # kn # knative client
    # kube-linter
    # kube3d
    # kubectl
    # kubernetes-helm
    nats-server
    natscli
    nixfmt
    nodejs-18_x # for Pulumi, must match values in package.json
    # openssl
    protoc-gen-go
    protoc-gen-go-grpc
    pulumi-bin
    saml2aws
    # zlib
  ] ++ lib.optionals stdenv.isDarwin [
    darwin.apple_sdk.frameworks.CoreServices
  ];
}
