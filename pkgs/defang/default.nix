# This file was generated by GoReleaser. DO NOT EDIT.
# vim: set ft=nix ts=2 sw=2 sts=2 et sta
{
system ? builtins.currentSystem
, lib
, fetchurl
, installShellFiles
, stdenvNoCC
, unzip
}:
let
  shaMap = {
    x86_64-linux = "0q6zqnxni9vh73fjmsfk6ppbh4m9dwj1mvxin1gxwnbcj9hsywd6";
    aarch64-linux = "04km33v76z3w5jyhrxrlkvgaya97yqbljm5g27rcwf7xxj3grlaf";
    x86_64-darwin = "02dr4nbbq5k3kqw0jgj51r2bk78nkxaz35wqp34y471r427vc1ly";
    aarch64-darwin = "02dr4nbbq5k3kqw0jgj51r2bk78nkxaz35wqp34y471r427vc1ly";
  };

  urlMap = {
    x86_64-linux = "https://github.com/DefangLabs/defang/releases/download/v0.5.19/defang_0.5.19_linux_amd64.tar.gz";
    aarch64-linux = "https://github.com/DefangLabs/defang/releases/download/v0.5.19/defang_0.5.19_linux_arm64.tar.gz";
    x86_64-darwin = "https://github.com/DefangLabs/defang/releases/download/v0.5.19/defang_0.5.19_macOS.zip";
    aarch64-darwin = "https://github.com/DefangLabs/defang/releases/download/v0.5.19/defang_0.5.19_macOS.zip";
  };
in
stdenvNoCC.mkDerivation {
  pname = "defang";
  version = "0.5.19";
  src = fetchurl {
    url = urlMap.${system};
    sha256 = shaMap.${system};
  };

  sourceRoot = ".";

  nativeBuildInputs = [ installShellFiles unzip ];

  installPhase = ''
    mkdir -p $out/bin
    cp -vr ./defang $out/bin/defang
  '';
  postInstall = ''
    installShellCompletion --cmd defang \
    --bash <($out/bin/defang completion bash) \
    --zsh <($out/bin/defang completion zsh) \
    --fish <($out/bin/defang completion fish)
  '';

  system = system;

  meta = {
    description = "Defang is easiest way for developers to create and deploy their containerized applications";
    homepage = "https://defang.io/";
    license = lib.licenses.mit;

    sourceProvenance = [ lib.sourceTypes.binaryNativeCode ];

    platforms = [
      "aarch64-darwin"
      "aarch64-linux"
      "x86_64-darwin"
      "x86_64-linux"
    ];
  };
}
