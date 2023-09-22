{ buildGoModule, lib }:
buildGoModule {
  pname = "defang-cli";
  version = "git";
  src = ../fabric;
  vendorHash = "sha256-vRVM1Y865B/XMHOXKKck4qSSDBJA47UxGQ92qdzQFAE=";

  subPackages = [ "cmd/cli" ];

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
