{ buildGoModule, lib }:
buildGoModule {
  pname = "defang-cli";
  version = "git";
  src = ../../src;
  vendorHash = "sha256-uxPeGtKw/DZwWZwFHuNloMc2LIsy9s/nsmHFttbMKC8=";

  subPackages = [ "cmd/cli" ];

  doCheck = false; # some unit tests need internet access

  postInstall = ''
    mv $out/bin/cli $out/bin/defang
  '';

  meta = with lib; {
    description = "Command-line interface for the Defang Opinionated Platform";
    homepage = "https://defang.io/";
    license = licenses.mit;
    maintainers = with maintainers; [ lionello ];
  };
}
