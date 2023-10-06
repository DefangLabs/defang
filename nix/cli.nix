{ buildGoModule, lib }:
buildGoModule {
  pname = "defang-cli";
  version = "git";
  src = ../src;
  vendorHash = "sha256-QD7JIgVujX9UIBBTJCxQwo6fN3CKno16hp+nnIweC54=";

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
