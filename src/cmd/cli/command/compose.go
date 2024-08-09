package command

import (
	"context"
	"errors"
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

var composeUpCmd = &cobra.Command{
	Use:         "up",
	Annotations: authNeededAnnotation,
	Args:        cobra.NoArgs, // TODO: takes optional list of service names
	Short:       "Reads a Compose file and deploy a new project or update an existing project",
	RunE: func(cmd *cobra.Command, args []string) error {
		var force, _ = cmd.Flags().GetBool("force")
		var detach, _ = cmd.Flags().GetBool("detach")

		since := time.Now()
		deploy, project, err := cli.ComposeUp(cmd.Context(), client, force)
		if err != nil {
			if !errors.Is(err, types.ErrComposeFileNotFound) {
				return err
			}
			printDefangHint("To start a new project, do:", "new")
			return err
		}

		if len(deploy.Services) == 0 {
			return errors.New("no services being deployed")
		}

		printPlaygroundPortalServiceURLs(deploy.Services)

		if detach {
			term.Info("Detached.")
			return nil
		}

		tailCtx, cancelTail := context.WithCancelCause(cmd.Context())
		defer cancelTail(nil) // to cancel WaitServiceState and clean-up context

		errCompleted := errors.New("deployment succeeded") // tail canceled because of deployment completion
		const targetState = defangv1.ServiceState_DEPLOYMENT_COMPLETED
		targetStateReached := false

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
				targetStateReached = true
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

			// Tail got canceled; if it was by anything other than completion, prompt to show debugger
			if !errors.Is(context.Cause(tailCtx), errCompleted) {
				var failedServices []string
				var errDeploymentFailed cli.ErrDeploymentFailed
				if errors.As(context.Cause(tailCtx), &errDeploymentFailed) {
					term.Warn(errDeploymentFailed)
					failedServices = []string{errDeploymentFailed.Service}
				} else {
					term.Warn("Deployment is not finished. Service(s) might not be running.")
					// TODO: some services might be OK and we should only debug the ones that are not
				}

				if _, isPlayground := client.(*cliClient.PlaygroundClient); !nonInteractive && isPlayground {
					var aiDebug bool
					Track("Debug Prompted", P{"failedServices", failedServices}, P{"etag", deploy.Etag}, P{"reason", context.Cause(tailCtx)})
					if err := survey.AskOne(&survey.Confirm{
						Message: "Would you like to debug the deployment with AI?",
						Help:    "This will send logs and artifacts to our backend and attempt to diagnose the issue and provide a solution.",
					}, &aiDebug); err != nil {
						term.Debugf("failed to ask for AI debug: %v", err)
						Track("Debug Prompt Failed", P{"etag", deploy.Etag}, P{"reason", err})
					} else if aiDebug {
						Track("Debug Prompt Accepted", P{"etag", deploy.Etag})
						// Call the AI debug endpoint using the original command context (not the tailCtx which is canceled); HACK: cmd might be canceled too
						// TODO: use the WorkingDir of the failed service, might not be the project's root
						if err := cli.Debug(context.TODO(), client, deploy.Etag, project, failedServices); err != nil {
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
		if targetStateReached {
			for _, service := range deploy.Services {
				service.State = targetState
			}

			printEndpoints(deploy.Services)
		}

		term.Info("Done.")
		return nil
	},
}

var composeStartCmd = &cobra.Command{
	Use:         "start",
	Aliases:     []string{"deploy"},
	Annotations: authNeededAnnotation,
	Args:        cobra.NoArgs, // TODO: takes optional list of service names
	Short:       "Reads a Compose file and deploys services to the cluster",
	RunE: func(cmd *cobra.Command, args []string) error {
		var force, _ = cmd.Flags().GetBool("force")

		deploy, _, err := cli.ComposeUp(cmd.Context(), client, force)
		if err != nil {
			return err
		}

		printPlaygroundPortalServiceURLs(deploy.Services)
		printEndpoints(deploy.Services) // TODO: do this at the end

		command := "tail"
		if deploy.Etag != "" {
			command += " --etag " + deploy.Etag
		}
		printDefangHint("To track the update, do:", command)
		return nil
	},
}

var composeRestartCmd = &cobra.Command{
	Use:         "restart",
	Annotations: authNeededAnnotation,
	Args:        cobra.NoArgs, // TODO: takes optional list of service names
	Short:       "Reads a Compose file and restarts its services",
	RunE: func(cmd *cobra.Command, args []string) error {
		etag, err := cli.ComposeRestart(cmd.Context(), client)
		if err != nil {
			return err
		}
		term.Info("Restarted services with deployment ID", etag)
		return nil
	},
}

var composeStopCmd = &cobra.Command{
	Use:         "stop",
	Annotations: authNeededAnnotation,
	Args:        cobra.NoArgs, // TODO: takes optional list of service names
	Short:       "Reads a Compose file and stops its services",
	RunE: func(cmd *cobra.Command, args []string) error {
		etag, err := cli.ComposeStop(cmd.Context(), client)
		if err != nil {
			return err
		}
		term.Info("Stopped services with deployment ID", etag)
		return nil
	},
}

var composeDownCmd = &cobra.Command{
	Use:         "down",
	Aliases:     []string{"rm"},
	Annotations: authNeededAnnotation,
	Args:        cobra.NoArgs, // TODO: takes optional list of service names
	Short:       "Reads a Compose file and deprovisions its services",
	RunE: func(cmd *cobra.Command, args []string) error {
		var detach, _ = cmd.Flags().GetBool("detach")

		since := time.Now()
		etag, err := cli.ComposeDown(cmd.Context(), client)
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

var composeConfigCmd = &cobra.Command{
	Use:   "config",
	Args:  cobra.NoArgs, // TODO: takes optional list of service names
	Short: "Reads a Compose file and shows the generated config",
	RunE: func(cmd *cobra.Command, args []string) error {
		cli.DoDryRun = true // config is like start in a dry run
		// force=false to calculate the digest
		if _, _, err := cli.ComposeUp(cmd.Context(), client, false); !errors.Is(err, cli.ErrDryRun) {
			return err
		}
		return nil
	},
}

func setupComposeCommand() *cobra.Command {
	// Compose Command
	// composeCmd.Flags().Bool("compatibility", false, "Run compose in backward compatibility mode"); TODO: Implement compose option
	// composeCmd.Flags().String("env-file", "", "Specify an alternate environment file."); TODO: Implement compose option
	// composeCmd.Flags().Int("parallel", -1, "Control max parallelism, -1 for unlimited (default -1)"); TODO: Implement compose option
	// composeCmd.Flags().String("profile", "", "Specify a profile to enable"); TODO: Implement compose option
	// composeCmd.Flags().String("project-directory", "", "Specify an alternate working directory"); TODO: Implement compose option
	composeUpCmd.Flags().Bool("tail", false, "tail the service logs after updating") // obsolete, but keep for backwards compatibility
	_ = composeUpCmd.Flags().MarkHidden("tail")
	composeUpCmd.Flags().Bool("force", false, "force a build of the image even if nothing has changed")
	composeUpCmd.Flags().BoolP("detach", "d", false, "run in detached mode")
	composeCmd.AddCommand(composeUpCmd)
	composeCmd.AddCommand(composeConfigCmd)
	composeDownCmd.Flags().Bool("tail", false, "tail the service logs after deleting") // obsolete, but keep for backwards compatibility
	composeDownCmd.Flags().BoolP("detach", "d", false, "run in detached mode")
	_ = composeDownCmd.Flags().MarkHidden("tail")
	composeCmd.AddCommand(composeDownCmd)
	composeStartCmd.Flags().Bool("force", false, "force a build of the image even if nothing has changed")
	composeCmd.AddCommand(composeStartCmd)
	composeCmd.AddCommand(composeRestartCmd)
	composeCmd.AddCommand(composeStopCmd)
	// composeCmd.AddCommand(getServicesCmd) // like docker compose ls

	return composeCmd
}
