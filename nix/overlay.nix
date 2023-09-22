final: prev: {
  pulumi-bin =
    let
      data = import ./pulumi-bin-data.nix { };
    in
    # Ensure the overridden version of Pulumi is not older than the version in Nixpkgs.
    assert -1 != builtins.compareVersions data.version prev.pulumi-bin.version;
    prev.pulumi-bin.overrideAttrs (finalAttrs: previousAttrs: {
      version = data.version;
      srcs = map (x: prev.fetchurl x) data.pulumiPkgs.${prev.stdenv.hostPlatform.system};
      meta.platforms = builtins.attrNames data.pulumiPkgs;
    });
}
