{ pkgs ? import <nixpkgs> {} }:
with pkgs;
let
  CPU = if stdenv.hostPlatform.isAarch64 then "arm64" else "amd64";
  OS = if stdenv.hostPlatform.isDarwin then "macOS" else "${stdenv.hostPlatform.parsed.kernel.name}_${CPU}";
  EXT = if stdenv.hostPlatform.isLinux then "tar.gz" else "zip";
  version = "0.4.28";
  hashes = {
    macOS = "sha256-H5YB10Ok/jxYmssQ6I8oh0ckr/QoLbNXoZt/olZkCUc=";
    linux_amd64 = "sha256-Rakr0QyoXvX7HrKhlhzHhhGkIYRpHbCXoPjW6dbsAAU=";
    linux_arm64 = "sha256-qZnFYf1QpnANr1F3sh/xcE7F7Op4h/98Vv+hjZ+XcNw=";
  };
in stdenv.mkDerivation {
  pname = "defang-bin";
  inherit version;

  src = fetchzip {
    url = "https://github.com/defang-io/defang/releases/download/v${version}/defang_${version}_${OS}.${EXT}";
    hash = hashes.${OS} or (throw "missing hash for OS ${OS}");
  };

  dontConfigure = true;
  dontBuild = true;

  installPhase = ''
    runHook preInstall
    mkdir -p $out/bin
    mv defang $out/bin/
    runHook postInstall
  '';

  meta = with lib; {
    description = "Command-line interface for the Defang Opinionated Platform";
    homepage = "https://defang.io/";
    license = licenses.mit;
    maintainers = with maintainers; [ lionello ];
  };
}
