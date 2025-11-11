package command

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/dryrun"
	"github.com/DefangLabs/defang/src/pkg/logs"
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/track"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/bufbuild/connect-go"
	"github.com/spf13/cobra"
)

var logType = logs.LogTypeAll

func makeComposeUpCmd() *cobra.Command {
	composeUpCmd := &cobra.Command{
		Use:         "up",
		Aliases:     []string{"deploy"},
		Annotations: authNeededAnnotation,
		Args:        cobra.NoArgs, // TODO: takes optional list of service names
		Short:       "Reads a Compose file and deploy a new project or update an existing project",
		RunE: func(cmd *cobra.Command, args []string) error {
			var force, _ = cmd.Flags().GetBool("force")
			var detach, _ = cmd.Flags().GetBool("detach")
			var utc, _ = cmd.Flags().GetBool("utc")
			var waitTimeout, _ = cmd.Flags().GetInt("wait-timeout")

			if utc {
				cli.EnableUTCMode()
			}

			upload := compose.UploadModeDigest
			if force {
				upload = compose.UploadModeForce
			}

			since := time.Now()
			loader := configureLoader(cmd)

			ctx := cmd.Context()
			project, loadErr := loader.LoadProject(ctx)
			if loadErr != nil {
				if nonInteractive {
					return loadErr
				}

				term.Error("Cannot load project:", loadErr)
				project, err := loader.CreateProjectForDebug()
				if err != nil {
					return err
				}

				track.Evt("Debug Prompted", P("loadErr", loadErr))
				return cli.InteractiveDebugForClientError(ctx, client, project, loadErr)
			}

			provider, err := newProviderChecked(ctx, loader)
			if err != nil {
				return err
			}

			// Check if the user has permission to use the provider
			err = canIUseProvider(ctx, provider, project.Name, len(project.Services))
			if err != nil {
				return err
			}

			// Check if the project is already deployed and warn the user if they're deploying it elsewhere
			if resp, err := client.ListDeployments(ctx, &defangv1.ListDeploymentsRequest{
				Project: project.Name,
				Type:    defangv1.DeploymentType_DEPLOYMENT_TYPE_ACTIVE,
			}); err != nil {
				term.Debugf("ListDeployments failed: %v", err)
			} else if accountInfo, err := provider.AccountInfo(ctx); err != nil {
				term.Debugf("AccountInfo failed: %v", err)
			} else {
				samePlace := slices.ContainsFunc(resp.Deployments, func(dep *defangv1.Deployment) bool {
					// Old deployments may not have a region or account ID, so we check for empty values too
					return dep.Provider == providerID.Value() && (dep.ProviderAccountId == accountInfo.AccountID || dep.ProviderAccountId == "") && (dep.Region == accountInfo.Region || dep.Region == "")
				})
				if !samePlace && len(resp.Deployments) > 0 {
					if nonInteractive {
						term.Warnf("Project appears to be already deployed elsewhere. Use `defang deployments --project-name=%q` to view all deployments.", project.Name)
					} else {
						help := "Active deployments of this project:"
						for _, dep := range resp.Deployments {
							var providerId cliClient.ProviderID
							providerId.SetValue(dep.Provider)
							help += fmt.Sprintf("\n - %v", cliClient.AccountInfo{Provider: providerId, AccountID: dep.ProviderAccountId, Region: dep.Region})
						}
						var confirm bool
						if err := survey.AskOne(&survey.Confirm{
							Message: "This project appears to be already deployed elsewhere. Are you sure you want to continue?",
							Help:    help,
							Default: false,
						}, &confirm, survey.WithStdio(term.DefaultTerm.Stdio())); err != nil {
							return err
						} else if !confirm {
							return fmt.Errorf("deployment of project %q was canceled", project.Name)
						}
					}
				}
			}

			// Show a warning for any (managed) services that we cannot monitor
			var managedServices []string
			for _, service := range project.Services {
				if !cli.CanMonitorService(&service) {
					managedServices = append(managedServices, service.Name)
				}
			}
			if len(managedServices) > 0 {
				term.Warnf("Defang cannot monitor status of the following managed service(s): %v.\n   To check if the managed service is up, check the status of the service which depends on it.", managedServices)
			}

			deploy, project, err := cli.ComposeUp(ctx, project, client, provider, upload, mode)
			if err != nil {
				return handleComposeUpErr(ctx, err, project, provider)
			}

			if len(deploy.Services) == 0 {
				return errors.New("no services being deployed")
			}

			printPlaygroundPortalServiceURLs(deploy.Services)

			if detach {
				term.Info("Detached.")
				return nil
			}

			// show users the current streaming logs
			tailSource := "all services"
			if deploy.Etag != "" {
				tailSource = "deployment ID " + deploy.Etag
			}
			term.Info("Tailing logs for", tailSource, "; press Ctrl+C to detach:")

			tailOptions := newTailOptionsForDeploy(deploy.Etag, since, verbose)
			serviceStates, err := cli.TailAndMonitor(ctx, project, provider, time.Duration(waitTimeout)*time.Second, tailOptions)
			if err != nil {
				handleTailAndMonitorErr(ctx, err, client, cli.DebugConfig{
					Deployment: deploy.Etag,
					ModelId:    modelId,
					Project:    project,
					Provider:   provider,
					Since:      since,
				})
				return err
			}

			for _, service := range deploy.Services {
				service.State = serviceStates[service.Service.Name]
			}

			// Print the current service states of the deployment
			err = printServiceStatesAndEndpoints(deploy.Services)
			if err != nil {
				return err
			}

			term.Info("Done.")
			flushWarnings()
			return nil
		},
	}
	composeUpCmd.Flags().BoolP("detach", "d", false, "run in detached mode")
	composeUpCmd.Flags().Bool("force", false, "force a build of the image even if nothing has changed")
	composeUpCmd.Flags().Bool("utc", false, "show logs in UTC timezone (ie. TZ=UTC)")
	composeUpCmd.Flags().Bool("tail", false, "tail the service logs after updating") // obsolete, but keep for backwards compatibility
	_ = composeUpCmd.Flags().MarkHidden("tail")
	composeUpCmd.Flags().VarP(&mode, "mode", "m", fmt.Sprintf("deployment mode; one of %v", modes.AllDeploymentModes()))
	composeUpCmd.Flags().Bool("build", true, "build the image before starting the service") // docker-compose compatibility
	_ = composeUpCmd.Flags().MarkHidden("build")
	composeUpCmd.Flags().Bool("wait", true, "wait for services to be running|healthy") // docker-compose compatibility
	_ = composeUpCmd.Flags().MarkHidden("wait")
	composeUpCmd.Flags().Int("wait-timeout", -1, "maximum duration to wait for the project to be running|healthy") // docker-compose compatibility
	return composeUpCmd
}

func handleComposeUpErr(ctx context.Context, err error, project *compose.Project, provider cliClient.Provider) error {
	if errors.Is(err, types.ErrComposeFileNotFound) {
		// TODO: generate a compose file based on the current project
		printDefangHint("To start a new project, do:", "new")
	}

	if nonInteractive || errors.Is(err, byoc.ErrLocalPulumiStopped) {
		return err
	}

	if strings.Contains(err.Error(), "maximum number of projects") {
		if projectName, err2 := provider.RemoteProjectName(ctx); err2 == nil {
			term.Error("Error:", cliClient.PrettyError(err))
			if _, err := cli.InteractiveComposeDown(ctx, provider, projectName); err != nil {
				term.Debug("ComposeDown failed:", err)
				printDefangHint("To deactivate a project, do:", "compose down --project-name "+projectName)
			} else {
				// TODO: actually do the "compose up" (because that's what the user intended in the first place)
				printDefangHint("To try deployment again, do:", "compose up")
			}
			return nil
		}
		return err
	}

	term.Error("Error:", cliClient.PrettyError(err))
	track.Evt("Debug Prompted", P("composeErr", err))
	return cli.InteractiveDebugForClientError(ctx, client, project, err)
}

func handleTailAndMonitorErr(ctx context.Context, err error, client *cliClient.GrpcClient, debugConfig cli.DebugConfig) {
	var errDeploymentFailed cliClient.ErrDeploymentFailed
	if errors.As(err, &errDeploymentFailed) {
		// Tail got canceled because of deployment failure: prompt to show the debugger
		term.Warn(errDeploymentFailed)
		if errDeploymentFailed.Service != "" {
			debugConfig.FailedServices = []string{errDeploymentFailed.Service}
		}
		if nonInteractive {
			printDefangHint("To debug the deployment, do:", debugConfig.String())
		} else {
			track.Evt("Debug Prompted", P("failedServices", debugConfig.FailedServices), P("etag", debugConfig.Deployment), P("reason", errDeploymentFailed))

			// Call the AI debug endpoint using the original command context (not the tail ctx which is canceled)
			if nil != cli.InteractiveDebugDeployment(ctx, client, debugConfig) {
				// don't show this defang hint if debugging was successful
				tailOptions := newTailOptionsForDeploy(debugConfig.Deployment, debugConfig.Since, true)
				printDefangHint("To see the logs of the failed service, run: defang logs", tailOptions.String())
			}
		}
	}
}

func newTailOptionsForDeploy(deployment string, since time.Time, verbose bool) cli.TailOptions {
	return cli.TailOptions{
		Deployment: deployment,
		LogType:    logs.LogTypeAll,
		// TODO: Move this to playground provider GetDeploymentStatus
		EndEventDetectFunc: func(eventLog *defangv1.LogEntry) error {
			if eventLog.Service == "cd" && eventLog.Host == "pulumi" && deployment == eventLog.Etag {
				if strings.Contains(eventLog.Message, "Update succeeded in ") {
					return io.EOF
				} else if strings.Contains(eventLog.Message, "Update failed in ") {
					return errors.New(eventLog.Message)
				}
			}
			return nil
		},
		Raw:     false,
		Since:   since,
		Verbose: verbose,
	}
}

func flushWarnings() {
	if hasTty && term.HadWarnings() {
		fmt.Println("\n\u26A0\uFE0F Some warnings were seen during this command:")
		term.FlushWarnings()
	}
}

func makeComposeStartCmd() *cobra.Command {
	composeStartCmd := &cobra.Command{
		Use:         "start",
		Aliases:     []string{"deploy"},
		Annotations: authNeededAnnotation,
		Args:        cobra.NoArgs, // TODO: takes optional list of service names
		Hidden:      true,
		Short:       "Reads a Compose file and deploys services to the cluster",
		RunE: func(cmd *cobra.Command, args []string) error {
			return errors.New("command 'start' is deprecated, use 'up' instead")
		},
	}
	composeStartCmd.Flags().Bool("force", false, "force a build of the image even if nothing has changed")
	return composeStartCmd
}

func makeComposeRestartCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "restart",
		Annotations: authNeededAnnotation,
		Args:        cobra.NoArgs, // TODO: takes optional list of service names
		Hidden:      true,
		Short:       "Reads a Compose file and restarts its services",
		RunE: func(cmd *cobra.Command, args []string) error {
			return errors.New("command 'restart' is deprecated, use 'up' instead")
		},
	}
}

func makeComposeStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "stop",
		Annotations: authNeededAnnotation,
		Args:        cobra.NoArgs, // TODO: takes optional list of service names
		Hidden:      true,
		Short:       "Reads a Compose file and stops its services",
		RunE: func(cmd *cobra.Command, args []string) error {
			return errors.New("command 'stop' is deprecated, use 'down' instead")
		},
	}
}

func makeComposeDownCmd() *cobra.Command {
	composeDownCmd := &cobra.Command{
		Use:         "down [SERVICE...]",
		Aliases:     []string{"rm", "remove"}, // like docker stack
		Annotations: authNeededAnnotation,
		Short:       "Reads a Compose file and deprovisions its services",
		RunE: func(cmd *cobra.Command, args []string) error {
			var detach, _ = cmd.Flags().GetBool("detach")
			var utc, _ = cmd.Flags().GetBool("utc")

			if utc {
				cli.EnableUTCMode()
			}

			loader := configureLoader(cmd)
			provider, err := newProviderChecked(cmd.Context(), loader)
			if err != nil {
				return err
			}

			projectName, err := cliClient.LoadProjectNameWithFallback(cmd.Context(), loader, provider)
			if err != nil {
				return err
			}

			err = canIUseProvider(cmd.Context(), provider, projectName, 0)
			if err != nil {
				return err
			}

			since := time.Now()
			deployment, err := cli.ComposeDown(cmd.Context(), projectName, client, provider, args...)
			if err != nil {
				if connect.CodeOf(err) == connect.CodeNotFound {
					// Show a warning (not an error) if the service was not found
					term.Warn(cliClient.PrettyError(err))
					return nil
				}
				return err
			}

			term.Info("Deleted services, deployment ID", deployment)

			listConfigs, err := provider.ListConfig(cmd.Context(), &defangv1.ListConfigsRequest{Project: projectName})
			if err == nil {
				if len(listConfigs.Names) > 0 {
					term.Warn("Stored project configs are not deleted.")
				}
			} else {
				term.Debugf("ListConfigs failed: %v", err)
			}

			if detach {
				printDefangHint("To track the update, do:", "tail --project-name="+projectName+" --deployment="+deployment)
				return nil
			}

			tailOptions := newTailOptionsForDown(deployment, since)
			tailCtx := cmd.Context() // FIXME: stop Tail when the deployment task is done
			err = cli.TailAndWaitForCD(tailCtx, provider, projectName, tailOptions)
			if err != nil && !errors.Is(err, io.EOF) {
				if connect.CodeOf(err) == connect.CodePermissionDenied {
					// If tail fails because of missing permission, we show a warning and detach. This is
					// different than `up`, which will wait for the deployment to finish, but we don't have an
					// ECS event subscription for `down` so we can't wait for the deployment to finish.
					// Instead, we'll just show a warning and detach.
					term.Warn("Unable to tail logs. Detaching.")
					return nil
				}
				return err
			}
			term.Info("Done.")
			if len(listConfigs.Names) > 0 {
				printDefangHint("To delete stored project configs, run:", "config rm --project-name="+projectName+" "+strings.Join(listConfigs.Names, " "))
			}
			return nil
		},
	}
	composeDownCmd.Flags().BoolP("detach", "d", false, "run in detached mode")
	composeDownCmd.Flags().Bool("utc", false, "show logs in UTC timezone (ie. TZ=UTC)")
	composeDownCmd.Flags().Bool("tail", false, "tail the service logs after deleting") // obsolete, but keep for backwards compatibility
	_ = composeDownCmd.Flags().MarkHidden("tail")
	return composeDownCmd
}

func newTailOptionsForDown(deployment string, since time.Time) cli.TailOptions {
	return cli.TailOptions{
		Deployment: deployment,
		Since:      since,
		// TODO: Move this to playground provider GetDeploymentStatus
		EndEventDetectFunc: func(eventLog *defangv1.LogEntry) error {
			if eventLog.Service == "cd" && eventLog.Host == "pulumi" && deployment == eventLog.Etag {
				if strings.Contains(eventLog.Message, "Destroy succeeded in ") || strings.Contains(eventLog.Message, "Update succeeded in ") {
					return io.EOF
				} else if strings.Contains(eventLog.Message, "Destroy failed in ") || strings.Contains(eventLog.Message, "Update failed in ") {
					return errors.New(eventLog.Message)
				}
			}
			return nil // keep tailing logs
		},
		Verbose: verbose,
		LogType: logs.LogTypeAll,
	}
}

func makeComposeConfigCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "config",
		Args:  cobra.NoArgs, // TODO: takes optional list of service names
		Short: "Reads a Compose file and shows the generated config",
		RunE: func(cmd *cobra.Command, args []string) error {
			loader := configureLoader(cmd)

			ctx := cmd.Context()
			project, loadErr := loader.LoadProject(ctx)
			if loadErr != nil {
				if nonInteractive {
					return loadErr
				}

				term.Error("Cannot load project:", loadErr)
				project, err := loader.CreateProjectForDebug()
				if err != nil {
					return err
				}

				track.Evt("Debug Prompted", P("loadErr", loadErr))
				return cli.InteractiveDebugForClientError(ctx, client, project, loadErr)
			}

			provider, err := newProvider(ctx, loader)
			if err != nil {
				return err
			}

			_, _, err = cli.ComposeUp(ctx, project, client, provider, compose.UploadModeIgnore, modes.ModeUnspecified)
			if !errors.Is(err, dryrun.ErrDryRun) {
				return err
			}
			return nil
		},
	}
}

func makeComposePsCmd() *cobra.Command {
	getServicesCmd := &cobra.Command{
		Use:         "ps",
		Annotations: authNeededAnnotation,
		Args:        cobra.NoArgs,
		Aliases:     []string{"getServices", "services"},
		Short:       "Get list of services in the project",
		RunE: func(cmd *cobra.Command, args []string) error {
			long, _ := cmd.Flags().GetBool("long")

			loader := configureLoader(cmd)
			provider, err := newProviderChecked(cmd.Context(), loader)
			if err != nil {
				return err
			}

			projectName, err := cliClient.LoadProjectNameWithFallback(cmd.Context(), loader, provider)
			if err != nil {
				return err
			}

			if err := cli.GetServices(cmd.Context(), projectName, provider, long); err != nil {
				if errNoServices := new(cli.ErrNoServices); !errors.As(err, errNoServices) {
					return err
				}

				term.Warn(err)
				printDefangHint("To start a new project, do:", "new")
				return nil
			}

			if !long {
				printDefangHint("To see more information about your services, do:", cmd.CalledAs()+" -l")
			}
			return nil
		},
	}
	getServicesCmd.Flags().BoolP("long", "l", false, "show more details")
	return getServicesCmd
}

func makeLogsCmd() *cobra.Command {
	var logsCmd = &cobra.Command{
		Use:         "logs [SERVICE...]",
		Annotations: authNeededAnnotation,
		Short:       "Show logs from one or more services",
		RunE:        handleLogsCmd,
	}
	setupLogsFlags(logsCmd)
	logsCmd.Flags().Int32("limit", 100, "maximum number of log lines to show")
	return logsCmd
}

func makeTailCmd() *cobra.Command {
	var tailCmd = &cobra.Command{
		Use:         "tail [SERVICE...]",
		Annotations: authNeededAnnotation,
		Short:       "Show logs from one or more services",
		RunE:        handleLogsCmd,
	}
	setupLogsFlags(tailCmd)
	tailCmd.Flags().Set("follow", "true")
	return tailCmd
}

func setupLogsFlags(cmd *cobra.Command) {
	cmd.Flags().StringP("name", "n", "", "name of the service (backwards compat)")
	cmd.Flags().MarkHidden("name")
	cmd.Flags().String("etag", "", "deployment ID (ETag) of the service")
	cmd.Flags().MarkHidden("etag")
	cmd.Flags().String("deployment", "", "deployment ID of the service")
	cmd.Flags().Bool("follow", false, "follow log output; incompatible with --until") // NOTE: -f is already used by --file
	cmd.Flags().BoolP("raw", "r", false, "show raw (unparsed) logs")
	cmd.Flags().String("since", "", "show logs since duration/time")
	cmd.Flags().String("until", "", "show logs until duration/time; incompatible with --follow")
	cmd.Flags().Bool("utc", false, "show logs in UTC timezone (ie. TZ=UTC)")
	cmd.Flags().Var(&logType, "type", fmt.Sprintf("show logs of type; one of %v", logs.AllLogTypes))
	cmd.Flags().String("filter", "", "only show logs containing given text; case-insensitive")
}

func handleLogsCmd(cmd *cobra.Command, args []string) error {
	var name, _ = cmd.Flags().GetString("name")
	var etag, _ = cmd.Flags().GetString("etag")
	var deployment, _ = cmd.Flags().GetString("deployment")
	var raw, _ = cmd.Flags().GetBool("raw")
	var since, _ = cmd.Flags().GetString("since")
	var utc, _ = cmd.Flags().GetBool("utc")
	var verbose, _ = cmd.Flags().GetBool("verbose")
	var filter, _ = cmd.Flags().GetString("filter")
	var until, _ = cmd.Flags().GetString("until")
	var follow, _ = cmd.Flags().GetBool("follow")
	var limit, _ = cmd.Flags().GetInt32("limit")

	if follow && until != "" {
		return errors.New("cannot use --follow and --until together")
	}

	if etag != "" && deployment == "" {
		deployment = etag
	}

	if utc {
		cli.EnableUTCMode()
	}

	if !cmd.Flags().Changed("verbose") {
		verbose = true // default verbose for explicit tail command
	}

	now := time.Now()
	sinceTs, err := cli.ParseTimeOrDuration(since, now)
	if err != nil {
		return fmt.Errorf("invalid 'since' duration or time: %w", err)
	}
	sinceTs = sinceTs.UTC()
	untilTs, err := cli.ParseTimeOrDuration(until, now)
	if err != nil {
		return fmt.Errorf("invalid 'until' duration or time: %w", err)
	}
	untilTs = untilTs.UTC()

	rangeStr := ""
	if pkg.IsValidTime(sinceTs) {
		rangeStr = " since " + sinceTs.Format(time.RFC3339Nano)
	}
	if pkg.IsValidTime(untilTs) {
		rangeStr += " until " + untilTs.Format(time.RFC3339Nano)
	}
	term.Infof("Showing logs%s; press Ctrl+C to stop:", rangeStr)

	services := args
	if len(name) > 0 {
		services = append(args, strings.Split(name, ",")...) // backwards compat
	}

	loader := configureLoader(cmd)
	provider, err := newProviderChecked(cmd.Context(), loader)
	if err != nil {
		return err
	}

	projectName, err := cliClient.LoadProjectNameWithFallback(cmd.Context(), loader, provider)
	if err != nil {
		return err
	}

	tailOptions := cli.TailOptions{
		Deployment: deployment,
		Filter:     filter,
		LogType:    logType,
		Raw:        raw,
		Services:   services,
		Since:      sinceTs,
		Until:      untilTs,
		Verbose:    verbose,
		Follow:     follow,
		Limit:      limit,
	}
	return cli.Tail(cmd.Context(), provider, projectName, tailOptions)
}

func setupComposeCommand() *cobra.Command {
	var composeCmd = &cobra.Command{
		Use:   "compose",
		Args:  cobra.NoArgs,
		Short: "Work with local Compose files",
		Long: `Define and deploy multi-container applications with Defang. Most compose commands require
a "compose.yaml" file. The simplest "compose.yaml" file with a single service is:

services:
  app:              # the name of the service
    build: .        # the folder with the Dockerfile and app sources (. means current folder)
    ports:
      - 80          # the port the service listens on for HTTP requests
`,
	}
	// Compose Command
	// composeCmd.Flags().Bool("compatibility", false, "Run compose in backward compatibility mode"); TODO: Implement compose option
	// composeCmd.Flags().String("env-file", "", "Specify an alternate environment file."); TODO: Implement compose option
	// composeCmd.Flags().Int("parallel", -1, "Control max parallelism, -1 for unlimited (default -1)"); TODO: Implement compose option
	// composeCmd.Flags().String("profile", "", "Specify a profile to enable"); TODO: Implement compose option
	// composeCmd.Flags().String("project-directory", "", "Specify an alternate working directory"); TODO: Implement compose option
	composeCmd.PersistentFlags().StringVar(&byoc.DefangPulumiBackend, "pulumi-backend", "", `specify an alternate Pulumi backend URL or "pulumi-cloud"`)
	composeCmd.AddCommand(makeComposeUpCmd())
	composeCmd.AddCommand(makeComposeConfigCmd())
	composeCmd.AddCommand(makeComposeDownCmd())
	composeCmd.AddCommand(makeComposePsCmd())
	composeCmd.AddCommand(makeLogsCmd())
	composeTailCmd := makeTailCmd()
	composeTailCmd.Hidden = true
	composeCmd.AddCommand(composeTailCmd)

	// deprecated, will be removed in future releases
	composeCmd.AddCommand(makeComposeStartCmd())
	composeCmd.AddCommand(makeComposeRestartCmd())
	composeCmd.AddCommand(makeComposeStopCmd())

	return composeCmd
}
