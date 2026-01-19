package command

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"
	"sync"
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
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

const DEFANG_PORTAL_HOST = "portal.defang.io"
const SERVICE_PORTAL_URL = "https://" + DEFANG_PORTAL_HOST + "/service"

type deploymentModel struct {
	services map[string]*serviceState
	quitting bool
	updateCh chan serviceUpdate
}

type serviceState struct {
	status  string
	spinner spinner.Model
}

type serviceUpdate struct {
	name   string
	status string
}

var (
	spinnerStyle = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#bc9724", Dark: "#2ddedc"})
	statusStyle  = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#a4729d", Dark: "#fae856"})
	nameStyle    = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#305897", Dark: "#cdd2c9"})
)

func newDeploymentModel(serviceNames []string) *deploymentModel {
	services := make(map[string]*serviceState)

	for _, name := range serviceNames {
		s := spinner.New()
		s.Spinner = spinner.Dot
		s.Style = spinnerStyle

		services[name] = &serviceState{
			status:  "DEPLOYMENT_QUEUED",
			spinner: s,
		}
	}

	return &deploymentModel{
		services: services,
		updateCh: make(chan serviceUpdate, 100),
	}
}

func (m *deploymentModel) Init() tea.Cmd {
	var cmds []tea.Cmd
	for _, svc := range m.services {
		cmds = append(cmds, svc.spinner.Tick)
	}
	return tea.Batch(cmds...)
}

func (m *deploymentModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			m.quitting = true
			return m, tea.Quit
		}
	case serviceUpdate:
		if svc, exists := m.services[msg.name]; exists {
			svc.status = msg.status
		}
		return m, nil
	case spinner.TickMsg:
		var cmds []tea.Cmd
		for _, svc := range m.services {
			var cmd tea.Cmd
			svc.spinner, cmd = svc.spinner.Update(msg)
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)
	}
	return m, nil
}

func (m *deploymentModel) View() string {
	if m.quitting {
		return ""
	}

	var lines []string
	// Sort services by name for consistent ordering
	var serviceNames []string
	for name := range m.services {
		serviceNames = append(serviceNames, name)
	}
	sort.Strings(serviceNames)

	for _, name := range serviceNames {
		svc := m.services[name]

		// Stop spinner for completed services
		var spinnerOrCheck string
		switch svc.status {
		case "DEPLOYMENT_COMPLETED":
			spinnerOrCheck = "✓ "
		case "DEPLOYMENT_FAILED":
			spinnerOrCheck = "✗ "
		default:
			spinnerOrCheck = svc.spinner.View()
		}

		line := lipgloss.JoinHorizontal(
			lipgloss.Left,
			spinnerOrCheck,
			" ",
			nameStyle.Render("["+name+"]"),
			" ",
			statusStyle.Render(svc.status),
		)
		lines = append(lines, line)
	}

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func monitorWithUI(ctx context.Context, project *compose.Project, provider client.Provider, waitTimeout time.Duration, deploymentID string) (map[string]defangv1.ServiceState, error) {
	servicesNames := make([]string, 0, len(project.Services))
	for _, svc := range project.Services {
		servicesNames = append(servicesNames, svc.Name)
	}

	// Initialize the bubbletea model
	model := newDeploymentModel(servicesNames)

	// Create the bubbletea program
	p := tea.NewProgram(model)

	var (
		serviceStates map[string]defangv1.ServiceState
		monitorErr    error
		wg            sync.WaitGroup
	)
	wg.Add(2) // One for UI, one for monitoring

	// Start the bubbletea UI in a goroutine
	go func() {
		defer wg.Done()
		if _, err := p.Run(); err != nil {
			// Handle UI errors if needed
		}
	}()

	// Start monitoring in a goroutine
	go func() {
		defer wg.Done()
		serviceStates, monitorErr = cli.Monitor(ctx, project, provider, waitTimeout, deploymentID, func(msg *defangv1.SubscribeResponse, states *cli.ServiceStates) error {
			// Send service status updates to the bubbletea model
			for name, state := range *states {
				p.Send(serviceUpdate{
					name:   name,
					status: state.String(),
				})
			}
			return nil
		})
		// empty out all of the service statuses before printing a final state
		for _, name := range servicesNames {
			p.Send(serviceUpdate{
				name:   name,
				status: "",
			})
		}
		// Quit the UI when monitoring is done
		p.Quit()
	}()

	wg.Wait()

	return serviceStates, monitorErr
}

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
		Aliases:     []string{"deploy"},
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
			sessionLoader := session.NewSessionLoader(global.Client, ec, sm, options)
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

				debugger, err := debug.NewDebugger(ctx, global.Cluster, &global.Stack)
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
				Stack:   global.Stack.Name,
			}); err != nil {
				term.Debugf("ListDeployments failed: %v", err)
			} else if accountInfo, err := session.Provider.AccountInfo(ctx); err != nil {
				term.Debugf("AccountInfo failed: %v", err)
			} else if len(resp.Deployments) > 0 {
				confirmed, err := confirmDeployment(session.Loader.TargetDirectory(), resp.Deployments, accountInfo, session.Provider.GetStackName())
				if err != nil {
					return err
				}
				if !confirmed {
					return fmt.Errorf("deployment of project %q was canceled", project.Name)
				}
			} else if global.Stack.Name == "" {
				err = promptToCreateStack(ctx, session.Loader.TargetDirectory(), stacks.Parameters{
					Name:     stacks.MakeDefaultName(accountInfo.Provider, accountInfo.Region),
					Provider: accountInfo.Provider,
					Region:   accountInfo.Region,
					Mode:     global.Stack.Mode,
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
				Mode:       global.Stack.Mode,
			})
			if err != nil {
				composeErr := err
				debugger, err := debug.NewDebugger(ctx, global.Cluster, &global.Stack)
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
			tailOptions := cli.TailOptions{
				Deployment: deploy.Etag,
				LogType:    logs.LogTypeAll,
				Since:      since,
				Verbose:    true,
			}

			waitTimeoutDuration := time.Duration(waitTimeout) * time.Second
			var serviceStates map[string]defangv1.ServiceState
			if global.Verbose || global.NonInteractive {
				tailOptions.Follow = true
				serviceStates, err = cli.TailAndMonitor(ctx, project, session.Provider, waitTimeoutDuration, tailOptions)
				if err != nil {
					return err
				}
			} else {
				term.Info("Live tail logs with `defang tail --deployment=" + deploy.Etag + "`")
				serviceStates, err = monitorWithUI(ctx, project, session.Provider, waitTimeoutDuration, deploy.Etag)
			}
			if err != nil && !errors.Is(err, context.Canceled) {
				deploymentErr := err

				// if any services failed to build, only show build logs for those
				// services
				var unbuiltServices = make([]string, 0, len(project.Services))
				for service, state := range serviceStates {
					if state <= defangv1.ServiceState_BUILD_STOPPING {
						unbuiltServices = append(unbuiltServices, service)
					}
				}
				if len(unbuiltServices) > 0 {
					tailOptions.LogType = logs.LogTypeBuild
					tailOptions.Services = unbuiltServices
				}
				err := cli.Tail(ctx, session.Provider, project.Name, tailOptions)
				if err != nil && !errors.Is(err, io.EOF) {
					term.Warn("Failed to tail logs for deployment error", err)
					return deploymentErr
				}

				debugger, err := debug.NewDebugger(ctx, global.Cluster, &global.Stack)
				if err != nil {
					term.Warn("Failed to initialize debugger:", err)
					return deploymentErr
				}
				handleTailAndMonitorErr(ctx, deploymentErr, debugger, debug.DebugConfig{
					Deployment: deploy.Etag,
					Project:    project,
					ProviderID: &session.Stack.Provider,
					Stack:      &session.Stack.Name,
					Since:      since,
					Until:      time.Now(),
				})
				return deploymentErr
			}

			for _, service := range deploy.Services {
				service.State = serviceStates[service.Service.Name]
			}

			// Print the current service states of the deployment
			if err := cli.PrintServices(cmd.Context(), project.Name, session.Provider); err != nil {
				term.Warn(err)
			}

			term.Info("Done.")
			flushWarnings()
			return nil
		},
	}
	composeUpCmd.Flags().BoolP("detach", "d", false, "run in detached mode")
	composeUpCmd.Flags().Bool("force", false, "force a build of the image even if nothing has changed")
	composeUpCmd.Flags().MarkDeprecated("force", "superseded by --build") // but keep for backwards compatibility
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
		return (dep.ProviderAccountId == accountInfo.AccountID || dep.ProviderAccountId == "") && (dep.Region == accountInfo.Region || dep.Region == "")
	})
	if samePlace {
		return true, nil
	}
	confirmed, err := confirmDeploymentToNewLocation(existingDeployments)
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
			term.Info(stacks.PostCreateMessage(stackName))
		}
	}
	return true, nil
}

func printExistingDeployments(existingDeployments []*defangv1.Deployment) {
	term.Info("This project has already deployed to the following locations:")
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

func confirmDeploymentToNewLocation(existingDeployments []*defangv1.Deployment) (bool, error) {
	printExistingDeployments(existingDeployments)
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

	term.Info(stacks.PostCreateMessage(params.Name))

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
			tailOptions := newTailOptionsForDeploy(debugConfig.Deployment, debugConfig.Since, true)
			printDefangHint("To see the logs of the failed service, run:", "logs "+tailOptions.String())
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
	if global.HasTty && term.HadWarnings() {
		term.Println("\n\u26A0\uFE0F Some warnings were seen during this command:")
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

			tailOptions := newTailOptionsForDown(deployment, since)
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
				RequireStack:     false, // for `compose config` it's OK to proceed without a stack
			})
			if err != nil {
				return fmt.Errorf("loading session: %w", err)
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
			Stack:   global.Stack.Name,
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
	composeTailCmd := makeTailCmd()
	composeTailCmd.Hidden = true
	composeCmd.AddCommand(composeTailCmd)

	// deprecated, will be removed in future releases
	composeCmd.AddCommand(makeComposeStartCmd())
	composeCmd.AddCommand(makeComposeRestartCmd())
	composeCmd.AddCommand(makeComposeStopCmd())

	return composeCmd
}
