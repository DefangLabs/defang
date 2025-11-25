{
  buildGo124Module,
  installShellFiles,
  lib,
}:
buildGo124Module {
  pname = "defang-cli";
  version = "git";
  src = lib.cleanSource ../../src;
  vendorHash = "sha256-le/O0WhOXO8ItgiG1ZRqDzMNV/2mOwWATn8T/xfn6dM="; # TODO: use fetchFromGitHub

  subPackages = [ "cmd/cli" ];

  nativeBuildInputs = [ installShellFiles ];

  env.CGO_ENABLED = 0;
  ldflags = [
    "-s"
    "-w"
  ];
  doCheck = false; # some unit tests need internet access

  postInstall = ''
    mv $out/bin/cli $out/bin/defang
    installShellCompletion --cmd defang \
      --bash <($out/bin/defang completion bash) \
      --zsh <($out/bin/defang completion zsh) \
      --fish <($out/bin/defang completion fish)
  '';

  meta = with lib; {
    description = "CLI to take your app from Docker Compose to a secure and scalable deployment on your favorite cloud in minutes";
    homepage = "https://defang.io/";
    license = licenses.mit;
    maintainers = with maintainers; [ lionello ];
    mainProgram = "defang";
  };
}
