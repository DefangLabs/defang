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
    x86_64-linux = "0jws9kf9x52h9pky67c8vvwv35vcxpyaa1c7dr39in5k1q77zysb";
    aarch64-linux = "1vm8zv34cb55czzl2fq7dggkjavc2zhm0wi2s27wkxyc7h2a44lb";
    x86_64-darwin = "0czpmlp4171bzzb27hg15snik785aypjk84ik5p20hnm9fdh6c3s";
    aarch64-darwin = "0czpmlp4171bzzb27hg15snik785aypjk84ik5p20hnm9fdh6c3s";
  };

  urlMap = {
    x86_64-linux = "https://github.com/DefangLabs/defang/releases/download/v0.6.1/defang_0.6.1_linux_amd64.tar.gz";
    aarch64-linux = "https://github.com/DefangLabs/defang/releases/download/v0.6.1/defang_0.6.1_linux_arm64.tar.gz";
    x86_64-darwin = "https://github.com/DefangLabs/defang/releases/download/v0.6.1/defang_0.6.1_macOS.zip";
    aarch64-darwin = "https://github.com/DefangLabs/defang/releases/download/v0.6.1/defang_0.6.1_macOS.zip";
  };
in
stdenvNoCC.mkDerivation {
  pname = "defang";
  version = "0.6.1";
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
    description = "Defang is the easiest way for developers to create and deploy their containerized applications";
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
