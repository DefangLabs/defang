package byoc

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/dns"
	"github.com/DefangLabs/defang/src/pkg/term"
)

const (
	CdTaskPrefix = "defang-cd" // WARNING: renaming this practically deletes the Pulumi state
)

var (
	DefangPulumiBackend    = os.Getenv("DEFANG_PULUMI_BACKEND") // FIXME: allow override in .defang file
	ErrLocalPulumiStopped  = errors.New("local pulumi command succeeded; stopping")
	PulumiConfigPassphrase = pkg.Getenv("PULUMI_CONFIG_PASSPHRASE", "asdf") // FIXME: allow override in .defang file
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
	term.Debug("Running local command `", cmd, "` in dir ", dir)
	// TODO - use enums to define commands instead of passing strings down from the caller
	// #nosec G204
	command := exec.CommandContext(ctx, cmd[0], cmd[1:]...)
	command.Dir = dir
	command.Env = env
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	return command.Run()
}

func DebugPulumiNodeJS(ctx context.Context, env []string, cmd ...string) error {
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
	return ErrLocalPulumiStopped
}

func DebugPulumiGolang(ctx context.Context, env []string, cmd ...string) error {
	localCmd := append([]string{"go", "run", "./..."}, cmd...)
	term.Debug(strings.Join(append(env, localCmd...), " "))

	dir := os.Getenv("DEFANG_PULUMI_DIR")
	if dir == "" {
		return nil // show the shell command, but use regular Pulumi command in cloud task
	}

	if gopath, err := exec.Command("go", "env", "GOPATH").Output(); err != nil {
		return err
	} else {
		env = append(env, "GOPATH="+strings.TrimSpace(string(gopath)))
	}

	// Run the Pulumi command locally
	env = append([]string{
		"PATH=" + os.Getenv("PATH"),
		"USER=" + pkg.GetCurrentUser(), // needed for Pulumi
		"HOME=" + os.Getenv("HOME"),    // needed for go
	}, env...)
	if err := runLocalCommand(ctx, filepath.Join(dir, "cd", "gcp"), env, localCmd...); err != nil {
		return err
	}
	// We always return an error to stop the CLI from "tailing" the cloud logs
	return ErrLocalPulumiStopped
}

func GetPrivateDomain(projectName string) string {
	return dns.SafeLabel(projectName) + ".internal" // FIXME: should contain stack and/or tenant names
}
