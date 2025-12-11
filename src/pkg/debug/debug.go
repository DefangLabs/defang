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
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/track"
	"github.com/DefangLabs/defang/src/pkg/types"
)

var P = track.P

type DebugConfig struct {
	ProviderID     *client.ProviderID
	Stack          *string
	Deployment     types.ETag
	FailedServices []string
	Project        *compose.Project
	Since          time.Time
	Until          time.Time
}

func (dc DebugConfig) String() string {
	cmd := "debug"
	if dc.Deployment != "" {
		cmd += " --deployment=" + dc.Deployment
	}
	if !dc.Since.IsZero() {
		cmd += " --since=" + dc.Since.UTC().Format(time.RFC3339Nano)
	}
	if !dc.Until.IsZero() {
		cmd += " --until=" + dc.Until.UTC().Format(time.RFC3339Nano)
	}
	if dc.Project != nil {
		cmd += " --project-name=" + dc.Project.Name
		if dc.Project.WorkingDir != "" {
			cmd += " --cwd=" + dc.Project.WorkingDir
		}
	}
	if len(dc.FailedServices) > 0 {
		cmd += " " + strings.Join(dc.FailedServices, " ")
	}
	// TODO: do we need to add --provider= or rely on the Fabric-supplied value?
	return cmd
}

type Surveyor interface {
	AskOne(q survey.Prompt, response interface{}, opts ...survey.AskOpt) error
}

type surveyor struct{}

func (s *surveyor) AskOne(q survey.Prompt, response interface{}, opts ...survey.AskOpt) error {
	return survey.AskOne(q, response, opts...)
}

type DebugAgent interface {
	StartWithMessage(ctx context.Context, prompt string) error
}

type Debugger struct {
	agent    DebugAgent
	surveyor Surveyor
}

func NewDebugger(ctx context.Context, addr string, providerID *client.ProviderID, stack *string) (*Debugger, error) {
	agent, err := agent.New(ctx, addr, providerID, stack)
	if err != nil {
		return nil, err
	}
	return &Debugger{
		agent:    agent,
		surveyor: &surveyor{},
	}, nil
}

func (d *Debugger) DebugDeployment(ctx context.Context, debugConfig DebugConfig) error {
	if debugConfig.Deployment == "" {
		return errors.New("no information to use for debugger")
	}
	return d.promptAndTrackDebugSession(func() error {
		return d.agent.StartWithMessage(ctx, buildDeploymentDebugPrompt(debugConfig))
	}, "Debug Deployment", P("etag", debugConfig.Deployment))
}

func (d *Debugger) DebugDeploymentError(ctx context.Context, debugConfig DebugConfig, deployErr error) error {
	return d.promptAndTrackDebugSession(func() error {
		prompt := buildDeploymentDebugPrompt(debugConfig) + " The error encountered was: " + deployErr.Error()
		return d.agent.StartWithMessage(ctx, prompt)
	}, "Debug Deployment Error", P("etag", debugConfig.Deployment), P("deployErr", deployErr))
}

func (d *Debugger) DebugComposeLoadError(ctx context.Context, debugConfig DebugConfig, loadErr error) error {
	return d.promptAndTrackDebugSession(func() error {
		prompt := "The following error occurred while loading the compose file. Help troubleshoot and recommend a solution.\n\n" + loadErr.Error()
		return d.agent.StartWithMessage(ctx, prompt)
	}, "Debug Load", P("etag", debugConfig.Deployment), P("composeErr", loadErr))
}

func (d *Debugger) promptAndTrackDebugSession(fn func() error, eventName string, eventProperty ...track.Property) error {
	track.Evt("Debug Prompted", eventProperty...)
	track.Evt(eventName+" Prompted", eventProperty...)
	aiDebug, err := d.promptForPermission()
	if err != nil {
		track.Evt(eventName+" Prompt Failed", append([]track.Property{P("reason", err)}, eventProperty...)...)
		return err
	}
	if !aiDebug {
		track.Evt(eventName+" Prompt Skipped", eventProperty...)
		return nil
	}
	track.Evt(eventName+" Prompt Accepted", eventProperty...)

	err = fn()
	if err != nil {
		return err
	}

	good, err := d.promptForFeedback()
	if err != nil {
		track.Evt(eventName+" Feedback Prompt Failed", append([]track.Property{P("reason", err)}, eventProperty...)...)
		return err
	}
	track.Evt(eventName+" Feedback Prompt Answered", append([]track.Property{P("feedback", good)}, eventProperty...)...)
	return nil
}

func (d *Debugger) promptForPermission() (bool, error) {
	var aiDebug bool
	err := d.surveyor.AskOne(&survey.Confirm{
		Message: "Would you like to debug the deployment with AI?",
		Help:    "This will send logs and artifacts to our backend and attempt to diagnose the issue and provide a solution.",
	}, &aiDebug, survey.WithStdio(term.DefaultTerm.Stdio()))
	if err != nil {
		return false, err
	}

	return aiDebug, err
}

func (d *Debugger) promptForFeedback() (bool, error) {
	var good bool
	err := d.surveyor.AskOne(&survey.Confirm{
		Message: "Was the debugging helpful?",
		Help:    "Please provide feedback to help us improve the debugging experience.",
	}, &good, survey.WithStdio(term.DefaultTerm.Stdio()))
	if err != nil {
		return false, err
	}

	return good, err
}

func buildDeploymentDebugPrompt(debugConfig DebugConfig) string {
	prompt := "An error occurred while deploying this project"
	if debugConfig.ProviderID == nil {
		prompt += " with Defang."
	} else {
		prompt += fmt.Sprintf(" to %s with Defang.", debugConfig.ProviderID.Name())
	}

	prompt += " Help troubleshoot and recommend a solution. Look at the logs to understand what happened."

	if debugConfig.Deployment != "" {
		prompt += fmt.Sprintf(" The deployment ID is %q.", debugConfig.Deployment)
	}

	if len(debugConfig.FailedServices) > 0 {
		prompt += fmt.Sprintf(" The services that failed to deploy are: %v.", debugConfig.FailedServices)
	}
	if pkg.IsValidTime(debugConfig.Since) {
		prompt += fmt.Sprintf(" The deployment started at %s.", debugConfig.Since.String())
	}
	if pkg.IsValidTime(debugConfig.Until) {
		prompt += fmt.Sprintf(" The deployment finished at %s.", debugConfig.Until.String())
	}

	if debugConfig.Project != nil {
		yaml, err := debugConfig.Project.MarshalYAML()
		if err != nil {
			term.Println("Failed to marshal compose project to YAML for debug:", err)
		}
		prompt += fmt.Sprintf(
			"The compose files are at %s. The compose file is as follows:\n\n%s",
			debugConfig.Project.ComposeFiles,
			yaml,
		)
	}
	return prompt
}
