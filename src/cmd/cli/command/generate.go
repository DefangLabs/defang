package command

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/setup"
	"github.com/DefangLabs/defang/src/pkg/surveyor"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/spf13/cobra"
)

var generateCmd = &cobra.Command{
	Use:     "generate",
	Args:    cobra.MaximumNArgs(1),
	Aliases: []string{"gen"},
	Short:   "Generate a sample Defang project",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()

		if global.NonInteractive {
			if len(args) == 0 {
				return errors.New("cannot run in non-interactive mode")
			}
			return cli.InitFromSamples(ctx, args[0], args)
		}

		setupClient := setup.SetupClient{
			Surveyor:   surveyor.NewDefaultSurveyor(),
			ModelID:    global.ModelID,
			Fabric:     global.Client,
			FabricAddr: global.FabricAddr,
		}

		var sample string
		if len(args) > 0 {
			sample = args[0]
		}
		result, err := setupClient.CloneSample(ctx, sample)
		if err != nil {
			return err
		}
		afterGenerate(ctx, result)
		return nil
	},
}

func afterGenerate(ctx context.Context, result setup.SetupResult) {
	term.Info("Code generated successfully in folder", result.Folder)
	editor := pkg.Getenv("DEFANG_EDITOR", "code") // TODO: should we use EDITOR env var instead? But won't handle terminal editors like vim
	cmdd := exec.Command(editor, result.Folder)
	err := cmdd.Start()
	if err != nil {
		term.Debugf("unable to launch editor %q: %v", editor, err)
	}

	cd := ""
	if result.Folder != "." {
		cd = "`cd " + result.Folder + "` and "
	}

	// Load the project and check for empty environment variables
	loader := compose.NewLoader(compose.WithPath(filepath.Join(result.Folder, "compose.yaml")))
	project, err := loader.LoadProject(ctx)
	if err != nil {
		term.Debugf("unable to load new project: %v", err)
	}

	var envInstructions []string
	for _, envVar := range collectUnsetEnvVars(project) {
		envInstructions = append(envInstructions, "config create "+envVar)
	}

	if len(envInstructions) > 0 {
		printDefangHint("Check the files in your favorite editor.\nTo configure this project, run "+cd, envInstructions...)
	} else {
		printDefangHint("Check the files in your favorite editor.\nTo deploy this project, run "+cd, "compose up")
	}
}
