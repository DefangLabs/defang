package command

import (
	"fmt"
	"os"
	"time"

	"github.com/DefangLabs/defang/src/pkg/cli"
	"github.com/DefangLabs/defang/src/pkg/debug"
	"github.com/DefangLabs/defang/src/pkg/timeutils"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/proto"
)

var debugCmd = &cobra.Command{
	Use:         "debug [SERVICE...]",
	Annotations: authNeededAnnotation,
	Hidden:      true,
	Short:       "Debug a build, deployment, or service failure",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		etag, _ := cmd.Flags().GetString("etag")
		deployment, _ := cmd.Flags().GetString("deployment")
		since, _ := cmd.Flags().GetString("since")
		until, _ := cmd.Flags().GetString("until")

		var projupdate defangv1.ProjectUpdate
		if b, err := os.ReadFile("/Users/llunesu/Downloads/zdqxyk3xati8"); err == nil {
			if err := proto.Unmarshal(b, &projupdate); err == nil {
				cli.PrintObject("", &projupdate)
			}
		}

		if etag != "" && deployment == "" {
			deployment = etag
		}

		session, err := newCommandSession(cmd)
		if err != nil {
			return err
		}

		project, err := session.Loader.LoadProject(ctx)
		if err != nil {
			return err
		}

		debugger, err := debug.NewDebugger(ctx, global.Cluster, session.Stack)
		if err != nil {
			return err
		}

		now := time.Now()
		sinceTs, err := timeutils.ParseTimeOrDuration(since, now)
		if err != nil {
			return fmt.Errorf("invalid 'since' time: %w", err)
		}
		untilTs, err := timeutils.ParseTimeOrDuration(until, now)
		if err != nil {
			return fmt.Errorf("invalid 'until' time: %w", err)
		}

		debugConfig := debug.DebugConfig{
			Deployment:     deployment,
			FailedServices: args,
			Project:        project,
			ProviderID:     &session.Stack.Provider,
			Stack:          session.Stack.Name,
			Since:          sinceTs.UTC(),
			Until:          untilTs.UTC(),
		}
		return debugger.DebugDeployment(ctx, debugConfig)
	},
}
