package command

import (
	"errors"

	"github.com/DefangLabs/defang/src/pkg/cli"
	"github.com/DefangLabs/defang/src/pkg/migrate"
	"github.com/DefangLabs/defang/src/pkg/setup"
	"github.com/DefangLabs/defang/src/pkg/surveyor"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:     "init [SAMPLE]",
	Args:    cobra.MaximumNArgs(1),
	Aliases: []string{"new"},
	Short:   "Create a new Defang project from a sample",
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

		var result setup.SetupResult
		var err error
		if len(args) > 0 {
			result, err = setupClient.CloneSample(ctx, args[0])
		} else if from, ok := cmd.Flag("from").Value.(*migrate.SourcePlatform); ok && *from == migrate.SourcePlatformHeroku {
			result, err = setupClient.MigrateFromHeroku(ctx)
		} else {
			result, err = setupClient.Start(ctx)
		}
		if err != nil {
			return err
		}
		afterGenerate(ctx, result)
		return nil
	},
}
