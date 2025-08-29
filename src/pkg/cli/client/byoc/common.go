package byoc

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/dns"
	"github.com/DefangLabs/defang/src/pkg/term"
)

const (
	CdTaskPrefix = "defang-cd" // WARNING: renaming this practically deletes the Pulumi state
)

var (
	DefangPrefix           = pkg.Getenv("DEFANG_PREFIX", "Defang") // prefix for all resources created by Defang
	DefangPulumiBackend    = os.Getenv("DEFANG_PULUMI_BACKEND")
	PulumiConfigPassphrase = pkg.Getenv("PULUMI_CONFIG_PASSPHRASE", "asdf")
)

func getPulumiAccessToken() (string, error) {
	pat := os.Getenv("PULUMI_ACCESS_TOKEN")
	if pat == "" {
		// TODO: could consider parsing ~/.pulumi/credentials.json
		return "", errors.New("PULUMI_ACCESS_TOKEN must be set for Pulumi Cloud")
	}
	return pat, nil
}

func GetPulumiBackend(stateUrl string) (string, string, error) {
	switch strings.ToLower(DefangPulumiBackend) {
	case "pulumi-cloud":
		pat, err := getPulumiAccessToken()
		return "PULUMI_ACCESS_TOKEN", pat, err
	case "":
		return "PULUMI_BACKEND_URL", stateUrl, nil
	default:
		return "PULUMI_BACKEND_URL", DefangPulumiBackend, nil
	}
}

func runLocalCommand(ctx context.Context, dir string, env []string, cmd ...string) error {
	// TODO - use enums to define commands instead of passing strings down from the caller
	// #nosec G204
	command := exec.CommandContext(ctx, cmd[0], cmd[1:]...)
	command.Dir = dir
	command.Env = env
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	// Start the process and do not wait for it to finish
	return command.Start()
}

func DebugPulumi(ctx context.Context, env []string, cmd ...string) error {
	// Locally we use the "dev" script from package.json to run Pulumi commands, which uses ts-node
	localCmd := append([]string{"npm", "run", "dev"}, cmd...)
	term.Debug(strings.Join(append(env, localCmd...), " "))

	dir := os.Getenv("DEFANG_PULUMI_DIR")
	if dir == "" {
		return nil // show the shell command, but use regular Pulumi command in cloud task
	}

	// Run the Pulumi command locally
	env = append([]string{
		"PATH=" + os.Getenv("PATH"),
		"USER=" + pkg.GetCurrentUser(), // needed for Pulumi
	}, env...)
	if err := runLocalCommand(ctx, dir, env, localCmd...); err != nil {
		return err
	}
	return nil
}

func GetPrivateDomain(projectName string) string {
	return dns.SafeLabel(projectName) + ".internal"
}
