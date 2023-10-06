{ buildGoModule, lib }:
buildGoModule {
  pname = "defang-cli";
  version = "git";
  src = ../src;
  vendorHash = "sha256-xwuujVEJQGrOptQg/xo/LAylcZlAQtRCjlZpbSQVVk8=";

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
