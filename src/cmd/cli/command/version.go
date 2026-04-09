package command

import (
	"errors"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/cli"
	"github.com/DefangLabs/defang/src/pkg/github"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/spf13/cobra"
	"golang.org/x/mod/semver"
)

func isNewer(current, comparand string) bool {
	version, ok := normalizeVersion(current)
	if !ok {
		return false // development versions are always considered "latest"
	}
	return semver.Compare(version, comparand) < 0
}

func isGitRef(maybeVersion string) bool {
	return len(maybeVersion) >= 7 && !strings.Contains(maybeVersion, ".")
}

func normalizeVersion(maybeVersion string) (string, bool) {
	version := "v" + maybeVersion
	if semver.IsValid(version) && !isGitRef(maybeVersion) {
		return version, true
	}
	return maybeVersion, false // leave as is
}

func GetCurrentVersion() string {
	version, _ := normalizeVersion(RootCmd.Version)
	return version
}

var versionCmd = &cobra.Command{
	Use:     "version",
	Args:    cobra.NoArgs,
	Aliases: []string{"ver", "stat", "status"}, // for backwards compatibility
	Short:   "Get version information for the CLI and Fabric service",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		cliVersion := GetCurrentVersion()

		if global.Json {
			latestVersion, err := github.GetLatestReleaseTag(ctx)
			fabricVersion, err2 := cli.GetVersion(ctx, global.Client)
			info := struct {
				CLI    string `json:"cli"`
				Latest string `json:"latest,omitempty"`
				Fabric string `json:"fabric,omitempty"`
			}{cliVersion, latestVersion, fabricVersion}
			return errors.Join(err, err2, term.Table(info))
		}

		term.Printc(term.BrightCyan, "Defang CLI:    ")
		term.Println(cliVersion)

		term.Printc(term.BrightCyan, "Latest CLI:    ")
		ver, err := github.GetLatestReleaseTag(ctx)
		term.Println(ver)

		term.Printc(term.BrightCyan, "Defang Fabric: ")
		ver, err2 := cli.GetVersion(ctx, global.Client)
		term.Println(ver)
		return errors.Join(err, err2)
	},
}
