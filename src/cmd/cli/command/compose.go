package command

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/DefangLabs/defang/src/pkg/cli"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/bufbuild/connect-go"
	"github.com/spf13/cobra"
)

func isManagedService(service *defangv1.Service) bool {
	if service == nil {
		return false
	}

	return service.StaticFiles != nil || service.Redis != nil || service.Postgres != nil
}

func GetUnreferencedManagedResources(serviceInfos []*defangv1.ServiceInfo) []string {
	managedResources := make([]string, 0)
	for _, service := range serviceInfos {
		if isManagedService(service.Service) {
			managedResources = append(managedResources, service.Service.Name)
		}
	}

	return managedResources
}

func makeComposeUpCmd() *cobra.Command {
	mode := Mode(defangv1.DeploymentMode_DEVELOPMENT)
	composeUpCmd := &cobra.Command{
		Use:         "up",
		Annotations: authNeededAnnotation,
		Args:        cobra.NoArgs, // TODO: takes optional list of service names
		Short:       "Reads a Compose file and deploy a new project or update an existing project",
		RunE: func(cmd *cobra.Command, args []string) error {
			var force, _ = cmd.Flags().GetBool("force")
			var detach, _ = cmd.Flags().GetBool("detach")

			since := time.Now()
			deploy, project, err := cli.ComposeUp(cmd.Context(), client, force, mode.Value())

			if err != nil {
				if !nonInteractive && strings.Contains(err.Error(), "maximum number of projects") {
					if resp, err2 := client.GetServices(cmd.Context()); err2 == nil {
						term.Error("Error:", prettyError(err))
						if _, err := cli.InteractiveComposeDown(cmd.Context(), client, resp.Project); err != nil {
							term.Debug("ComposeDown failed:", err)
							printDefangHint("To deactivate a project, do:", "compose down --project-name "+resp.Project)
						} else {
							printDefangHint("To try deployment again, do:", "compose up")
						}
						return nil
					}
				}
				if errors.Is(err, types.ErrComposeFileNotFound) {
					printDefangHint("To start a new project, do:", "new")
				}
				return err
			}

			if len(deploy.Services) == 0 {
				return errors.New("no services being deployed")
			}

			printPlaygroundPortalServiceURLs(deploy.Services)

			var managedResources = GetUnreferencedManagedResources(deploy.Services)
			if len(managedResources) > 0 {
				term.Warnf("Defang cannot monitor status of the following managed service(s): %v.\n   To check if the managed service is up, check the status of the service which depends on it.", managedResources)
			}

			if detach {
				term.Info("Detached.")
				return nil
			}

			tailCtx, cancelTail := context.WithCancelCause(cmd.Context())
			defer cancelTail(nil) // to cancel WaitServiceState and clean-up context

			errCompleted := errors.New("deployment succeeded") // tail canceled because of deployment completion
			const targetState = defangv1.ServiceState_DEPLOYMENT_COMPLETED

			go func() {
				services := make([]string, len(deploy.Services))
				for i, serviceInfo := range deploy.Services {
					services[i] = serviceInfo.Service.Name
				}

				if err := cli.WaitServiceState(tailCtx, client, targetState, deploy.Etag, services); err != nil {
					var errDeploymentFailed cli.ErrDeploymentFailed
					if errors.As(err, &errDeploymentFailed) {
						cancelTail(err)
					} else if !errors.Is(err, context.Canceled) {
						term.Warnf("failed to wait for service status: %v", err) // TODO: don't print in Go-routine
					}
				} else {
					cancelTail(errCompleted)
				}
			}()

			// show users the current streaming logs
			tailSource := "all services"
			if deploy.Etag != "" {
				tailSource = "deployment ID " + deploy.Etag
			}

			term.Info("Tailing logs for", tailSource, "; press Ctrl+C to detach:")
			tailParams := cli.TailOptions{
				Etag:  deploy.Etag,
				Since: since,
				Raw:   false,
			}

			// blocking call to tail
			if err := cli.Tail(tailCtx, client, tailParams); err != nil {
				term.Debugf("Tail failed with %v", err)

				if connect.CodeOf(err) == connect.CodePermissionDenied {
					// If tail fails because of missing permission, we wait for the deployment to finish
					term.Warn("Unable to tail logs. Waiting for the deployment to finish.")
					<-tailCtx.Done()
				} else if !errors.Is(tailCtx.Err(), context.Canceled) {
					return err // any error other than cancelation
				}

				// The tail was canceled; check if it was because of deployment failure or explicit cancelation
				if errors.Is(context.Cause(tailCtx), context.Canceled) {
					// Tail was canceled by the user before deployment completion/failure; show a warning and exit with an error
					term.Warn("Deployment is not finished. Service(s) might not be running.")
					return err
				}

				var errDeploymentFailed cli.ErrDeploymentFailed
				if errors.As(context.Cause(tailCtx), &errDeploymentFailed) {
					// Tail got canceled because of deployment failure: prompt to show the debugger
					term.Warn(errDeploymentFailed)

					if _, isPlayground := client.(*cliClient.PlaygroundClient); !nonInteractive && isPlayground {
						failedServices := []string{errDeploymentFailed.Service}
						Track("Debug Prompted", P{"failedServices", failedServices}, P{"etag", deploy.Etag}, P{"reason", context.Cause(tailCtx)})
						var aiDebug bool
						if err := survey.AskOne(&survey.Confirm{
							Message: "Would you like to debug the deployment with AI?",
							Help:    "This will send logs and artifacts to our backend and attempt to diagnose the issue and provide a solution.",
						}, &aiDebug); err != nil {
							term.Debugf("failed to ask for AI debug: %v", err)
							Track("Debug Prompt Failed", P{"etag", deploy.Etag}, P{"reason", err})
						} else if aiDebug {
							Track("Debug Prompt Accepted", P{"etag", deploy.Etag})
							// Call the AI debug endpoint using the original command context (not the tailCtx which is canceled)
							if err := cli.Debug(cmd.Context(), client, deploy.Etag, project, failedServices); err != nil {
								term.Warnf("failed to debug deployment: %v", err)
							}
						} else {
							Track("Debug Prompt Skipped", P{"etag", deploy.Etag})
						}
					}
					return err
				}
			}

			// Print the current service states of the deployment
			if errors.Is(context.Cause(tailCtx), errCompleted) {
				for _, service := range deploy.Services {
					service.State = targetState
				}

				printEndpoints(deploy.Services)
			}

			term.Info("Done.")
			return nil
		},
	}
	composeUpCmd.Flags().BoolP("detach", "d", false, "run in detached mode")
	composeUpCmd.Flags().Bool("force", false, "force a build of the image even if nothing has changed")
	composeUpCmd.Flags().Bool("tail", false, "tail the service logs after updating") // obsolete, but keep for backwards compatibility
	_ = composeUpCmd.Flags().MarkHidden("tail")
	composeUpCmd.Flags().VarP(&mode, "mode", "m", "deployment mode, possible values: "+strings.Join(allModes(), ", "))
	composeUpCmd.Flags().Bool("build", true, "build the image before starting the service") // docker-compose compatibility
	_ = composeUpCmd.Flags().MarkHidden("build")
	return composeUpCmd
}

func makeComposeStartCmd() *cobra.Command {
	composeStartCmd := &cobra.Command{
		Use:         "start",
		Aliases:     []string{"deploy"},
		Annotations: authNeededAnnotation,
		Args:        cobra.NoArgs, // TODO: takes optional list of service names
		Deprecated:  "use `up` instead",
		Short:       "Reads a Compose file and deploys services to the cluster",
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
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
		Deprecated:  "use `up` instead",
		Short:       "Reads a Compose file and restarts its services",
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
}

func makeComposeStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "stop",
		Annotations: authNeededAnnotation,
		Args:        cobra.NoArgs, // TODO: takes optional list of service names
		Deprecated:  "use `down` instead",
		Short:       "Reads a Compose file and stops its services",
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
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

			since := time.Now()
			etag, err := cli.ComposeDown(cmd.Context(), client, "", args...)
			if err != nil {
				if connect.CodeOf(err) == connect.CodeNotFound {
					// Show a warning (not an error) if the service was not found
					term.Warn(prettyError(err))
					return nil
				}
				return err
			}

			term.Info("Deleted services, deployment ID", etag)

			if detach {
				printDefangHint("To track the update, do:", "tail --etag "+etag)
				return nil
			}

			endLogConditions := []cli.EndLogConditional{
				{Service: "cd", Host: "pulumi", EventLog: "Destroy succeeded in "},
				{Service: "cd", Host: "pulumi", EventLog: "Update succeeded in "},
			}

			endLogDetectFunc := cli.CreateEndLogEventDetectFunc(endLogConditions)
			tailParams := cli.TailOptions{
				Etag:               etag,
				Since:              since,
				Raw:                false,
				EndEventDetectFunc: endLogDetectFunc,
			}

			err = cli.Tail(cmd.Context(), client, tailParams)
			if err != nil {
				return err
			}
			term.Info("Done.")
			return nil

		},
	}
	composeDownCmd.Flags().BoolP("detach", "d", false, "run in detached mode")
	composeDownCmd.Flags().Bool("tail", false, "tail the service logs after deleting") // obsolete, but keep for backwards compatibility
	_ = composeDownCmd.Flags().MarkHidden("tail")
	return composeDownCmd
}

func makeComposeConfigCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "config",
		Args:  cobra.NoArgs, // TODO: takes optional list of service names
		Short: "Reads a Compose file and shows the generated config",
		RunE: func(cmd *cobra.Command, args []string) error {
			cli.DoDryRun = true // config is like start in a dry run
			// force=false to calculate the digest
			if _, _, err := cli.ComposeUp(cmd.Context(), client, false, defangv1.DeploymentMode_UNSPECIFIED_MODE); !errors.Is(err, cli.ErrDryRun) {
				return err
			}
			return nil
		},
	}
}

func makeComposeLsCmd() *cobra.Command {
	getServicesCmd := &cobra.Command{
		Use:         "ps",
		Annotations: authNeededAnnotation,
		Args:        cobra.NoArgs,
		Aliases:     []string{"getServices", "services"},
		Short:       "Get list of services in the project",
		RunE: func(cmd *cobra.Command, args []string) error {
			long, _ := cmd.Flags().GetBool("long")

			err := cli.GetServices(cmd.Context(), client, long)
			if err != nil {
				if !errors.Is(err, cli.ErrNoServices) {
					return err
				}

				term.Warn("No services found")

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

func makeComposeLogsCmd() *cobra.Command {
	var logsCmd = &cobra.Command{
		Use:         "logs",
		Annotations: authNeededAnnotation,
		Aliases:     []string{"tail"},
		Args:        cobra.NoArgs,
		Short:       "Tail logs from one or more services",
		RunE: func(cmd *cobra.Command, args []string) error {
			var name, _ = cmd.Flags().GetString("name")
			var etag, _ = cmd.Flags().GetString("etag")
			var raw, _ = cmd.Flags().GetBool("raw")
			var since, _ = cmd.Flags().GetString("since")
			var utc, _ = cmd.Flags().GetBool("utc")

			if utc {
				os.Setenv("TZ", "") // used by Go's "time" package, see https://pkg.go.dev/time#Location
			}

			ts, err := cli.ParseTimeOrDuration(since, time.Now())
			if err != nil {
				return fmt.Errorf("invalid duration or time: %w", err)
			}

			ts = ts.UTC()
			sinceStr := ""
			if !ts.IsZero() {
				sinceStr = " since " + ts.Format(time.RFC3339Nano) + " "
			}
			term.Infof("Showing logs%s; press Ctrl+C to stop:", sinceStr)
			services := []string{}
			if len(name) > 0 {
				services = strings.Split(name, ",")
			}
			tailOptions := cli.TailOptions{
				Services: services,
				Etag:     etag,
				Since:    ts,
				Raw:      raw,
			}

			return cli.Tail(cmd.Context(), client, tailOptions)
		},
	}
	logsCmd.Flags().StringP("name", "n", "", "name of the service")
	logsCmd.Flags().String("etag", "", "deployment ID (ETag) of the service")
	logsCmd.Flags().BoolP("raw", "r", false, "show raw (unparsed) logs")
	logsCmd.Flags().StringP("since", "S", "", "show logs since duration/time")
	logsCmd.Flags().Bool("utc", false, "show logs in UTC timezone (ie. TZ=UTC)")
	return logsCmd
}

func setupComposeCommand() *cobra.Command {
	var composeCmd = &cobra.Command{
		Use:     "compose",
		Aliases: []string{"stack"},
		Args:    cobra.NoArgs,
		Short:   "Work with local Compose files",
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
	composeCmd.AddCommand(makeComposeUpCmd())
	composeCmd.AddCommand(makeComposeConfigCmd())
	composeCmd.AddCommand(makeComposeDownCmd())
	composeCmd.AddCommand(makeComposeLsCmd())
	composeCmd.AddCommand(makeComposeLogsCmd())

	// deprecated, will be removed in future releases
	composeCmd.AddCommand(makeComposeStartCmd())
	composeCmd.AddCommand(makeComposeRestartCmd())
	composeCmd.AddCommand(makeComposeStopCmd())
	return composeCmd
}
