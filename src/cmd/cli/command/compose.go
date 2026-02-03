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
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/debug"
	"github.com/DefangLabs/defang/src/pkg/dryrun"
	"github.com/DefangLabs/defang/src/pkg/logs"
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/DefangLabs/defang/src/pkg/session"
	"github.com/DefangLabs/defang/src/pkg/stacks"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/timeutils"
	"github.com/DefangLabs/defang/src/pkg/track"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/bufbuild/connect-go"
	"github.com/spf13/cobra"
)

const DEFANG_PORTAL_HOST = "portal.defang.io"
const SERVICE_PORTAL_URL = "https://" + DEFANG_PORTAL_HOST + "/service"

func printPlaygroundPortalServiceURLs(serviceInfos []*defangv1.ServiceInfo) {
	// We can only show services deployed to the prod1 defang SaaS environment.
	if global.Stack.Provider == client.ProviderDefang && global.Cluster == client.DefaultCluster {
		term.Info("Monitor your services' status in the defang portal")
		for _, serviceInfo := range serviceInfos {
			term.Println("   -", SERVICE_PORTAL_URL+"/"+serviceInfo.Service.Name)
		}
	}
}

var logType = logs.LogTypeAll

func makeComposeUpCmd() *cobra.Command {
	composeUpCmd := &cobra.Command{
		Use:         "up",
		Aliases:     []string{"deploy"}, // Pulumi has "update" but it's ambiguous with "defang upgrade"
		Annotations: authNeededAnnotation,
		Args:        cobra.NoArgs, // TODO: takes optional list of service names
		Short:       "Reads a Compose file and deploy a new project or update an existing project",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			var build, _ = cmd.Flags().GetBool("build")
			var force, _ = cmd.Flags().GetBool("force")
			var detach, _ = cmd.Flags().GetBool("detach")
			var utc, _ = cmd.Flags().GetBool("utc")
			var waitTimeout, _ = cmd.Flags().GetInt("wait-timeout")

			if utc {
				cli.EnableUTCMode()
			}

			upload := compose.UploadModeDefault
			if force {
				upload = compose.UploadModeForce
			} else if build {
				upload = compose.UploadModeDigest
			}

			since := time.Now()

			options := newSessionLoaderOptionsForCommand(cmd)
			options.AllowStackCreation = true
			sm, err := newStackManagerForLoader(ctx, configureLoader(cmd))
			if err != nil {
				return err
			}
			sessionLoader := session.NewSessionLoader(global.Client, sm, options)
			session, err := sessionLoader.LoadSession(ctx)
			if err != nil {
				return err
			}

			project, loadErr := session.Loader.LoadProject(ctx)
			if loadErr != nil {
				if global.NonInteractive {
					return loadErr
				}

				term.Error("Cannot load project:", loadErr)
				project, err := session.Loader.CreateProjectForDebug()
				if err != nil {
					return err
				}

				debugger, err := debug.NewDebugger(ctx, global.Cluster, session.Stack)
				if err != nil {
					return err
				}
				return debugger.DebugComposeLoadError(ctx, debug.DebugConfig{
					Project: project,
				}, loadErr)
			}

			// Check if the user has permission to use the provider
			err = canIUseProvider(ctx, session.Provider, project.Name, len(project.Services))
			if err != nil {
				return err
			}

			// Check if the project is already deployed and warn the user if they're deploying it elsewhere
			if resp, err := global.Client.ListDeployments(ctx, &defangv1.ListDeploymentsRequest{
				Project: project.Name,
				Type:    defangv1.DeploymentType_DEPLOYMENT_TYPE_ACTIVE,
				Stack:   session.Stack.Name,
			}); err != nil {
				term.Debugf("ListDeployments failed: %v", err)
			} else if accountInfo, err := session.Provider.AccountInfo(ctx); err != nil {
				term.Debugf("AccountInfo failed: %v", err)
			} else if len(resp.Deployments) > 0 {
				confirmed, err := confirmDeployment(session.Loader.TargetDirectory(ctx), resp.Deployments, accountInfo, session.Provider.GetStackName())
				if err != nil {
					return err
				}
				if !confirmed {
					return fmt.Errorf("deployment of project %q was canceled", project.Name)
				}
			} else if session.Stack.Name == "" {
				err = promptToCreateStack(ctx, session.Loader.TargetDirectory(ctx), stacks.Parameters{
					Name:     stacks.MakeDefaultName(accountInfo.Provider, accountInfo.Region),
					Provider: accountInfo.Provider,
					Region:   accountInfo.Region,
					Mode:     session.Stack.Mode,
				})
				if err != nil {
					term.Debug("Failed to create stack:", err)
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

			deploy, project, err := cli.ComposeUp(ctx, global.Client, session.Provider, session.Stack, cli.ComposeUpParams{
				Project:    project,
				UploadMode: upload,
				Mode:       session.Stack.Mode,
			})
			if err != nil {
				composeErr := err
				debugger, err := debug.NewDebugger(ctx, global.Cluster, session.Stack)
				if err != nil {
					return err
				}
				return handleComposeUpErr(ctx, debugger, project, session.Provider, composeErr)
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

			tailOptions := newTailOptionsForDeploy(session.Stack.Name, deploy.Etag, since, global.Verbose)
			serviceStates, err := cli.TailAndMonitor(ctx, project, session.Provider, time.Duration(waitTimeout)*time.Second, tailOptions)
			if err != nil {
				deploymentErr := err
				debugger, err := debug.NewDebugger(ctx, global.Cluster, session.Stack)
				if err != nil {
					term.Warn("Failed to initialize debugger:", err)
					return deploymentErr
				}
				handleTailAndMonitorErr(ctx, deploymentErr, debugger, debug.DebugConfig{
					Deployment: deploy.Etag,
					Project:    project,
					ProviderID: &session.Stack.Provider,
					Stack:      session.Stack.Name,
					Since:      since,
					Until:      time.Now(),
				})
				return deploymentErr
			}

			for _, service := range deploy.Services {
				service.State = serviceStates[service.Service.Name]
			}

			services, err := cli.NewServiceFromServiceInfo(deploy.Services)
			if err != nil {
				return err
			}

			// Print the current service states of the deployment
			err = cli.PrintServiceStatesAndEndpoints(services)
			if err != nil {
				return err
			}

			term.Info("Done.")
			flushWarnings()
			return nil
		},
	}
	composeUpCmd.Flags().BoolP("detach", "d", false, "run in detached mode")
	composeUpCmd.Flags().Bool("force", false, "force a build of the image even if nothing has changed; implies --build")
	composeUpCmd.Flags().Bool("utc", false, "show logs in UTC timezone (ie. TZ=UTC)")
	composeUpCmd.Flags().Bool("tail", false, "tail the service logs after updating") // no-op, but keep for backwards compatibility
	_ = composeUpCmd.Flags().MarkHidden("tail")
	composeUpCmd.Flags().VarP(&global.Stack.Mode, "mode", "m", fmt.Sprintf("deployment mode; one of %v", modes.AllDeploymentModes()))
	composeUpCmd.Flags().Bool("build", false, "build images before starting services") // docker-compose compatibility
	composeUpCmd.Flags().Bool("wait", true, "wait for services to be running|healthy") // docker-compose compatibility
	_ = composeUpCmd.Flags().MarkHidden("wait")
	composeUpCmd.Flags().Int("wait-timeout", -1, "maximum duration to wait for the project to be running|healthy") // docker-compose compatibility
	return composeUpCmd
}

func confirmDeployment(targetDirectory string, existingDeployments []*defangv1.Deployment, accountInfo *client.AccountInfo, stackName string) (bool, error) {
	samePlace := slices.ContainsFunc(existingDeployments, func(dep *defangv1.Deployment) bool {
		if dep.Provider != accountInfo.Provider.Value() {
			return false
		}
		// Old deployments may not have a region or account ID, so we check for empty values too
		return (dep.ProviderAccountId == accountInfo.AccountID || dep.ProviderAccountId == "") && (dep.Region == accountInfo.Region || dep.Region == "") // already filtered by stack
	})
	if samePlace {
		return true, nil
	}
	printExistingDeployments(existingDeployments)
	if global.NonInteractive {
		return true, nil
	}
	confirmed, err := confirmDeploymentToNewLocation()
	if err != nil {
		return false, err
	}
	if !confirmed {
		return false, nil
	}
	if stackName == "" {
		stackName = stacks.DefaultBeta
		_, err := stacks.CreateInDirectory(targetDirectory, stacks.Parameters{
			Name:     stackName,
			Provider: accountInfo.Provider,
			Region:   accountInfo.Region,
			Mode:     global.Stack.Mode,
		})
		if err != nil {
			term.Debugf("Failed to create stack %v", err)
		} else {
			stacks.PrintCreateMessage(stackName)
		}
	}
	return true, nil
}

func printExistingDeployments(existingDeployments []*defangv1.Deployment) {
	term.Info("This project was previously deployed to the following locations:")
	deploymentStrings := make([]string, 0, len(existingDeployments))
	for _, dep := range existingDeployments {
		var providerId client.ProviderID
		providerId.SetValue(dep.Provider)
		deploymentStrings = append(deploymentStrings, fmt.Sprintf(" - %v", client.AccountInfo{Provider: providerId, AccountID: dep.ProviderAccountId, Region: dep.Region}))
	}
	// sort and remove duplicates
	slices.Sort(deploymentStrings)
	deploymentStrings = slices.Compact(deploymentStrings)
	term.Println(strings.Join(deploymentStrings, "\n"))
}

func confirmDeploymentToNewLocation() (bool, error) {
	var confirm bool
	if err := survey.AskOne(&survey.Confirm{
		Message: "Are you sure you want to continue?",
		Default: false,
	}, &confirm, survey.WithStdio(term.DefaultTerm.Stdio())); err != nil {
		return false, err
	} else if !confirm {
		return false, nil
	}
	return true, nil
}

func promptToCreateStack(ctx context.Context, targetDirectory string, params stacks.Parameters) error {
	if global.NonInteractive {
		term.Info("Consider creating a stack to manage your deployments.")
		printDefangHint("To create a stack, do:", "stack new --name="+params.Name)
		return nil
	}

	err := PromptForStackParameters(ctx, &params)
	if err != nil {
		return err
	}

	_, err = stacks.CreateInDirectory(targetDirectory, params)
	if err != nil {
		return err
	}

	stacks.PrintCreateMessage(params.Name)

	return nil
}

func handleComposeUpErr(ctx context.Context, debugger *debug.Debugger, project *compose.Project, provider client.Provider, originalErr error) error {
	if errors.Is(originalErr, types.ErrComposeFileNotFound) {
		// TODO: generate a compose file based on the current project
		printDefangHint("To start a new project, do:", "new")
	}

	if connect.CodeOf(originalErr) == connect.CodeResourceExhausted && strings.Contains(originalErr.Error(), "maximum number of projects") {
		term.Error("Error:", client.PrettyError(originalErr))
		err := handleTooManyProjectsError(ctx, provider, originalErr)
		if err != nil {
			return originalErr
		}
		return nil
	}

	if global.NonInteractive || errors.Is(originalErr, byoc.ErrLocalPulumiStopped) {
		return originalErr
	}

	term.Error("Error:", client.PrettyError(originalErr))
	return debugger.DebugDeploymentError(ctx, debug.DebugConfig{
		Project: project,
	}, originalErr)
}

func handleTooManyProjectsError(ctx context.Context, provider client.Provider, originalErr error) error {
	projectName, err := provider.RemoteProjectName(ctx)
	if err != nil {
		term.Warn("failed to get remote project name:", err)
		return originalErr
	}

	// print the error before prompting for compose down
	if global.NonInteractive {
		printDefangHint("To deactivate a project, do:", "compose down --project-name "+projectName)
		return originalErr
	}

	_, err = cli.InteractiveComposeDown(ctx, projectName, global.Client, provider)
	if err != nil {
		term.Warn("ComposeDown failed:", err)
		printDefangHint("To deactivate a project, do:", "compose down --project-name "+projectName)
		return originalErr
	} else {
		// TODO: actually do the "compose up" (because that's what the user intended in the first place)
		printDefangHint("To try deployment again, do:", "compose up")
	}

	return nil
}

func handleTailAndMonitorErr(ctx context.Context, err error, debugger *debug.Debugger, debugConfig debug.DebugConfig) {
	var errDeploymentFailed client.ErrDeploymentFailed
	if errors.As(err, &errDeploymentFailed) {
		// Tail got canceled because of deployment failure: prompt to show the debugger
		term.Warn(errDeploymentFailed)
		if errDeploymentFailed.Service != "" {
			debugConfig.FailedServices = []string{errDeploymentFailed.Service}
		}

		if global.NonInteractive {
			printDefangHint("To debug the deployment, do:", debugConfig.String())
			return
		}

		// Call the AI debug endpoint using the original command context (not the tail ctx which is canceled)
		if nil != debugger.DebugDeploymentError(ctx, debugConfig, errDeploymentFailed) {
			// don't show this defang hint if debugging was successful
			tailOptions := newTailOptionsForDeploy(debugConfig.Stack, debugConfig.Deployment, debugConfig.Since, true)
			printDefangHint("To see the logs of the failed service, run:", "logs "+tailOptions.String())
		}
	}
}

func newTailOptionsForDeploy(stack, deployment string, since time.Time, verbose bool) cli.TailOptions {
	return cli.TailOptions{
		Stack:      stack,
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
	if global.HasTty && term.HadWarnings() {
		term.Println("\n\u26A0\uFE0F Some warnings were seen during this command:")
		term.FlushWarnings()
	}
}

func makeComposeDownCmd() *cobra.Command {
	composeDownCmd := &cobra.Command{
		Use:         "down",
		Aliases:     []string{"rm", "remove"}, // like docker stack
		Annotations: authNeededAnnotation,
		Args:        cobra.NoArgs, // TODO: optional list of service names to remove select services
		Short:       "Reads a Compose file and deprovisions its services",
		RunE: func(cmd *cobra.Command, args []string) error {
			var detach, _ = cmd.Flags().GetBool("detach")
			var utc, _ = cmd.Flags().GetBool("utc")

			if utc {
				cli.EnableUTCMode()
			}

			session, err := newCommandSession(cmd)
			if err != nil {
				return err
			}

			projectName, err := client.LoadProjectNameWithFallback(cmd.Context(), session.Loader, session.Provider)
			if err != nil {
				return err
			}

			err = canIUseProvider(cmd.Context(), session.Provider, projectName, 0)
			if err != nil {
				return err
			}

			since := time.Now()
			deployment, err := cli.ComposeDown(cmd.Context(), projectName, global.Client, session.Provider)
			if err != nil {
				if connect.CodeOf(err) == connect.CodeNotFound {
					// Show a warning (not an error) if the service was not found
					term.Warn(client.PrettyError(err))
					return nil
				}
				return err
			}

			term.Info("Deleted services, deployment ID", deployment)

			listConfigs, err := session.Provider.ListConfig(cmd.Context(), &defangv1.ListConfigsRequest{Project: projectName})
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

			tailOptions := newTailOptionsForDown(session.Stack.Name, deployment, since)
			tailCtx := cmd.Context() // FIXME: stop Tail when the deployment task is done
			err = cli.TailAndWaitForCD(tailCtx, session.Provider, projectName, tailOptions)
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
	composeDownCmd.Flags().Bool("tail", false, "tail the service logs after deleting") // no-op, but keep for backwards compatibility
	_ = composeDownCmd.Flags().MarkHidden("tail")
	return composeDownCmd
}

func newTailOptionsForDown(stack, deployment string, since time.Time) cli.TailOptions {
	return cli.TailOptions{
		Stack:      stack,
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
		Verbose: global.Verbose,
		LogType: logs.LogTypeAll,
	}
}

func makeComposeConfigCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "config",
		Args:  cobra.NoArgs, // TODO: takes optional list of service names
		Short: "Reads a Compose file and shows the generated config",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			session, err := newCommandSessionWithOpts(cmd, commandSessionOpts{
				CheckAccountInfo: false,
			})
			if err != nil {
				term.Warn("unable to load stack:", err, "- some information may not be up-to-date")
			}

			_, err = session.Provider.AccountInfo(ctx)
			if err != nil {
				term.Warn("unable to connect to cloud provider:", err, "- some information may not be up-to-date")
			}

			project, loadErr := session.Loader.LoadProject(ctx)
			if loadErr != nil {
				if global.NonInteractive {
					return loadErr
				}

				term.Error("Cannot load project:", loadErr)
				project, err := session.Loader.CreateProjectForDebug()
				if err != nil {
					term.Warn("Failed to create project for debug:", err)
					return loadErr
				}

				track.Evt("Debug Prompted", P("loadErr", loadErr))
				debugger, err := debug.NewDebugger(ctx, global.Cluster, &global.Stack)
				if err != nil {
					term.Warn("Failed to initialize debugger:", err)
					return loadErr
				}
				return debugger.DebugComposeLoadError(ctx, debug.DebugConfig{
					Project: project,
				}, loadErr)
			}

			_, _, err = cli.ComposeUp(ctx, global.Client, session.Provider, session.Stack, cli.ComposeUpParams{
				Project:    project,
				UploadMode: compose.UploadModeIgnore,
				Mode:       modes.ModeUnspecified,
			})
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

			session, err := newCommandSession(cmd)
			if err != nil {
				return err
			}

			projectName, err := client.LoadProjectNameWithFallback(cmd.Context(), session.Loader, session.Provider)
			if err != nil {
				return err
			}

			if long {
				return cli.PrintLongServices(cmd.Context(), projectName, session.Provider)
			}

			if err := cli.PrintServices(cmd.Context(), projectName, session.Provider); err != nil {
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
	cmd.Flags().MarkDeprecated("etag", "superseded by --deployment") // but keep for backwards compatibility
	cmd.Flags().String("deployment", "", "deployment ID of the service (use 'latest' for the most recent deployment)")
	cmd.Flags().Bool("follow", false, "follow log output; incompatible with --until") // NOTE: -f is already used by --file
	cmd.Flags().BoolP("raw", "r", false, "show raw (unparsed) logs")
	cmd.Flags().String("since", "", "show logs since duration or timestamp (unix or RFC3339)")
	cmd.Flags().String("until", "", "show logs until duration or timestamp (unix or RFC3339); incompatible with --follow")
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
	sinceTs, err := timeutils.ParseTimeOrDuration(since, now)
	if err != nil {
		return fmt.Errorf("invalid 'since' duration or time: %w", err)
	}
	sinceTs = sinceTs.UTC()
	untilTs, err := timeutils.ParseTimeOrDuration(until, now)
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
	if logType.Has(logs.LogTypeBuild) {
		servicesWithBuild := make([]string, 0, len(services)*2)
		for _, service := range services {
			servicesWithBuild = append(servicesWithBuild, service)
			if !strings.HasSuffix(service, "-image") {
				servicesWithBuild = append(servicesWithBuild, service+"-image")
			}
		}
		services = servicesWithBuild
	}

	session, err := newCommandSession(cmd)
	if err != nil {
		return err
	}

	projectName, err := client.LoadProjectNameWithFallback(cmd.Context(), session.Loader, session.Provider)
	if err != nil {
		return err
	}

	// Handle 'latest' deployment flag
	if deployment == "latest" {
		resp, err := global.Client.ListDeployments(cmd.Context(), &defangv1.ListDeploymentsRequest{
			Project: projectName,
			Stack:   session.Stack.Name,
			Type:    defangv1.DeploymentType_DEPLOYMENT_TYPE_ACTIVE,
			Limit:   1,
		})
		if err != nil {
			return fmt.Errorf("failed to fetch latest deployment: %w", err)
		}
		if len(resp.Deployments) == 0 {
			return errors.New("no active deployments found")
		}
		deployment = resp.Deployments[0].Id
	}

	tailOptions := cli.TailOptions{
		Deployment:    deployment,
		Filter:        filter,
		LogType:       logType,
		Raw:           raw,
		Services:      services,
		Since:         sinceTs,
		Until:         untilTs,
		Verbose:       verbose,
		Follow:        follow,
		Limit:         limit,
		PrintBookends: true,
		Stack:         session.Stack.Name,
	}
	return cli.Tail(cmd.Context(), session.Provider, projectName, tailOptions)
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
	composeLsCmd := makeDeploymentsCmd("ls")
	composeCmd.AddCommand(composeLsCmd)
	composeTailCmd := makeTailCmd()
	composeTailCmd.Hidden = true
	composeCmd.AddCommand(composeTailCmd)
	return composeCmd
}
