{ buildGoModule
, installShellFiles
, lib
}:
buildGoModule {
  pname = "defang-cli";
  version = "git";
  src = ../../src;
  vendorHash = "sha256-4GF/EXgy6IoIq6AEUKn0YNRBpgI2c8pw6YiVtB/j+kc=";

  subPackages = [ "cmd/cli" ];

  nativeBuildInputs = [
    installShellFiles
  ];

  CGO_ENABLED = 0;
  GOFLAGS = [ "-trimpath" ];
  ldflags = [ "-s" "-w" ];
  doCheck = false; # some unit tests need internet access

  postInstall = ''
    mv $out/bin/cli $out/bin/defang
    installShellCompletion --cmd defang \
      --bash <($out/bin/defang completion bash) \
      --zsh <($out/bin/defang completion zsh) \
      --fish <($out/bin/defang completion fish)
  '';

  meta = with lib; {
    description = "Command-line interface for the Defang Opinionated Platform";
    homepage = "https://defang.io/";
    license = licenses.mit;
    maintainers = with maintainers; [ lionello ];
  };
}
