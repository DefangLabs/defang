package debug

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/agent"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/dryrun"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/track"
	"github.com/DefangLabs/defang/src/pkg/types"
)

var P = track.P

type DebugConfig struct {
	ProviderID     *client.ProviderID
	Deployment     types.ETag
	FailedServices []string
	ModelId        string
	Project        *compose.Project
	Provider       client.Provider
	Since          time.Time
	Until          time.Time
}

func (dc DebugConfig) String() string {
	cmd := "debug"
	if dc.Deployment != "" {
		cmd += " --deployment=" + dc.Deployment
	}
	if dc.ModelId != "" {
		cmd += " --model=" + dc.ModelId
	}
	if !dc.Since.IsZero() {
		cmd += " --since=" + dc.Since.UTC().Format(time.RFC3339Nano)
	}
	if !dc.Until.IsZero() {
		cmd += " --until=" + dc.Until.UTC().Format(time.RFC3339Nano)
	}
	if dc.Project.WorkingDir != "" {
		cmd += " --cwd=" + dc.Project.WorkingDir
	}
	if dc.Project != nil {
		cmd += " --project-name=" + dc.Project.Name
	}
	if len(dc.FailedServices) > 0 {
		cmd += " " + strings.Join(dc.FailedServices, " ")
	}
	// TODO: do we need to add --provider= or rely on the Fabric-supplied value?
	return cmd
}

func InteractiveDebugDeployment(ctx context.Context, addr string, debugConfig DebugConfig) error {
	return interactiveDebug(ctx, addr, debugConfig, nil)
}

func InteractiveDebugForClientError(ctx context.Context, addr string, debugConfig DebugConfig, clientErr error) error {
	return interactiveDebug(ctx, addr, debugConfig, clientErr)
}

func interactiveDebug(ctx context.Context, addr string, debugConfig DebugConfig, clientErr error) error {
	var aiDebug bool
	if err := survey.AskOne(&survey.Confirm{
		Message: "Would you like to debug the deployment with AI?",
		Help:    "This will send logs and artifacts to our backend and attempt to diagnose the issue and provide a solution.",
	}, &aiDebug, survey.WithStdio(term.DefaultTerm.Stdio())); err != nil {
		track.Evt("Debug Prompt Failed", P("etag", debugConfig.Deployment), P("reason", err), P("loadErr", clientErr))
		return err
	} else if !aiDebug {
		track.Evt("Debug Prompt Skipped", P("etag", debugConfig.Deployment), P("loadErr", clientErr))
		return err
	}

	track.Evt("Debug Prompt Accepted", P("etag", debugConfig.Deployment), P("loadErr", clientErr))

	if clientErr != nil {
		prompt := "The following error occurred while loading the compose file. Help troubleshoot and recommend a solution." + clientErr.Error()
		ag, err := agent.New(ctx, addr, debugConfig.ProviderID, agent.DefaultSystemPrompt)
		if err != nil {
			term.Warnf("Failed to debug compose file load: %v", err)
			return err
		}
		return ag.StartWithMessage(ctx, prompt)
	} else if debugConfig.Deployment != "" {
		if err := startDebugAgent(ctx, addr, debugConfig); err != nil {
			term.Warnf("Failed to start debug agent: %v", err)
		}
	} else {
		return errors.New("no information to use for debugger")
	}

	var goodBad bool
	if err := survey.AskOne(&survey.Confirm{
		Message: "Was the debugging helpful?",
		Help:    "Please provide feedback to help us improve the debugging experience.",
	}, &goodBad); err != nil {
		track.Evt("Debug Feedback Prompt Failed", P("etag", debugConfig.Deployment), P("reason", err), P("loadErr", clientErr))
	} else {
		track.Evt("Debug Feedback Prompt Answered", P("etag", debugConfig.Deployment), P("feedback", goodBad), P("loadErr", clientErr))
	}
	return nil
}

func DebugDeployment(ctx context.Context, addr string, debugConfig DebugConfig) error {
	term.Debugf("Invoking AI debugger for deployment %q", debugConfig.Deployment)

	if dryrun.DoDryRun {
		return dryrun.ErrDryRun
	}

	ag, err := agent.New(ctx, addr, debugConfig.ProviderID, agent.DefaultSystemPrompt)
	if err != nil {
		return err
	}

	prompt := fmt.Sprintf(
		"An error occurred while deploying this project to %s with Defang. "+
			"Help troubleshoot and recommend a solution. Look at the logs to understand what happened."+
			"The deployment ID is %q.", debugConfig.ProviderID.Name(), debugConfig.Deployment)

	if len(debugConfig.FailedServices) > 0 {
		prompt += fmt.Sprintf(" The services that failed to deploy are: %v.", debugConfig.FailedServices)
	}
	if pkg.IsValidTime(debugConfig.Since) {
		prompt += fmt.Sprintf(" The deployment started at %s.", debugConfig.Since.String())
	}
	if pkg.IsValidTime(debugConfig.Until) {
		prompt += fmt.Sprintf(" The deployment finished at %s.", debugConfig.Until.String())
	}

	return ag.StartWithMessage(ctx, prompt)
}

func startDebugAgent(ctx context.Context, addr string, debugConfig DebugConfig) error {
	term.Debug("Using Defang Agent for debugging")
	yaml, err := debugConfig.Project.MarshalYAML()
	if err != nil {
		term.Println("Failed to marshal compose project to YAML for debug:", err)
	}
	prompt := fmt.Sprintf(
		"An error occurred while deploying this project to %s with Defang. "+
			"Help troubleshoot and recommend a solution. Look at the logs to understand what happened."+
			"The deployment ID is %q.", debugConfig.ProviderID.Name(), debugConfig.Deployment)

	if len(debugConfig.FailedServices) > 0 {
		prompt += fmt.Sprintf(" The services that failed to deploy are: %v.", debugConfig.FailedServices)
	}
	if pkg.IsValidTime(debugConfig.Since) {
		prompt += fmt.Sprintf(" The deployment started at %s.", debugConfig.Since.String())
	}
	if pkg.IsValidTime(debugConfig.Until) {
		prompt += fmt.Sprintf(" The deployment finished at %s.", debugConfig.Until.String())
	}

	prompt += fmt.Sprintf(
		"The compose files are at %s. The compose file is as follows:\n\n%s",
		debugConfig.Project.ComposeFiles,
		yaml,
	)
	ag, err := agent.New(ctx, addr, debugConfig.ProviderID, agent.DefaultSystemPrompt)
	if err != nil {
		return err
	}
	return ag.StartWithMessage(ctx, prompt)
}
