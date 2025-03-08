package byoc

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"

	"github.com/DefangLabs/defang/src/pkg"
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

func GetPulumiBackend(stateUrl string) (string, string) {
	switch strings.ToLower(DefangPulumiBackend) {
	case "pulumi-cloud":
		return "PULUMI_ACCESS_TOKEN", os.Getenv("PULUMI_ACCESS_TOKEN")
	case "":
		return "PULUMI_BACKEND_URL", stateUrl
	default:
		return "PULUMI_BACKEND_URL", DefangPulumiBackend
	}
}

// This function was copied from Fabric controller and slightly modified to work with BYOC
func DnsSafeLabel(fqn string) string {
	return strings.ReplaceAll(DnsSafe(fqn), ".", "-")
}

func DnsSafe(fqdn string) string {
	return strings.ToLower(fqdn)
}

func runLocalCommand(ctx context.Context, dir string, env []string, cmd ...string) error {
	command := exec.CommandContext(ctx, cmd[0], cmd[1:]...)
	command.Dir = dir
	command.Env = env
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	return command.Run()
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
	// We always return an error to stop the CLI from "tailing" the cloud logs
	return errors.New("local pulumi command succeeded; stopping")
}

func GetPrivateDomain(projectName string) string {
	return DnsSafeLabel(projectName) + ".internal"
}
