package debug

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/AlecAivazis/survey/v2"
	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/agent"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/stacks"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/track"
	"github.com/DefangLabs/defang/src/pkg/types"
)

var P = track.P

// DebugOperation names the operation that failed. A failed teardown needs different advice from a
// failed deployment: its symptom is cloud resources left behind rather than a service that will
// not start, and the useful next step is the cleanup tool rather than the service logs.
type DebugOperation int

const (
	OperationDeploy DebugOperation = iota
	OperationTeardown
	OperationCleanup
)

type DebugConfig struct {
	Operation      DebugOperation
	ProviderID     *client.ProviderID
	Stack          string
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
	if dc.Stack != "" {
		cmd += " --stack=" + dc.Stack
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
	// interactive is false in environments without a user to prompt (e.g. CI). Callers decide
	// whether to build a debugger at all there; once built, it runs without prompting.
	interactive bool
}

func NewDebugger(ctx context.Context, fabricAddr string, stack *stacks.Parameters, interactive bool) (*Debugger, error) {
	var opts []agent.Option
	if !interactive {
		opts = append(opts, agent.WithNonInteractive())
	}
	agent, err := agent.New(ctx, fabricAddr, stack, opts...)
	if err != nil {
		return nil, err
	}
	return &Debugger{
		agent:       agent,
		surveyor:    &surveyor{},
		interactive: interactive,
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
		prompt := buildDeploymentDebugPrompt(debugConfig) + " The error encountered was: " + truncateTail(deployErr.Error(), maxPromptErrorLen)
		return d.agent.StartWithMessage(ctx, prompt)
	}, "Debug Deployment Error", P("etag", debugConfig.Deployment), P("deployErr", deployErr))
}

// DebugCleanupError hands a failed cleanup to the agent. Unlike DebugDeploymentError there is no
// deployment to read logs from: the report of what failed is the whole input.
func (d *Debugger) DebugCleanupError(ctx context.Context, debugConfig DebugConfig, cleanupErr error) error {
	return d.promptAndTrackDebugSession(func() error {
		prompt := buildDeploymentDebugPrompt(debugConfig) + " The cleanup reported: " + truncateTail(cleanupErr.Error(), maxPromptErrorLen)
		return d.agent.StartWithMessage(ctx, prompt)
	}, "Debug Cleanup Error", P("cleanupErr", cleanupErr))
}

func (d *Debugger) DebugComposeLoadError(ctx context.Context, debugConfig DebugConfig, loadErr error) error {
	return d.promptAndTrackDebugSession(func() error {
		prompt := "The following error occurred while loading the compose file. Help troubleshoot and recommend a solution.\n\n" + truncateTail(loadErr.Error(), maxPromptErrorLen)
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
	term.Warn("AI-generated analysis may be inaccurate. Please verify it against the logs.")

	if d.interactive {
		feedback, err := d.promptForFeedback()
		if err != nil {
			track.Evt(eventName+" Feedback Prompt Failed", append([]track.Property{P("reason", err)}, eventProperty...)...)
			return err
		}
		// The prompt tells the user their answer is sent to Defang, so the text is tracked, but it
		// is free-form: cap it so an accidental paste (a log dump, a stack trace) does not end up
		// in analytics wholesale.
		track.Evt(eventName+" Feedback Prompt Answered", append([]track.Property{P("feedback", truncateHead(feedback, maxFeedbackLen))}, eventProperty...)...)
	}
	return nil
}

func (d *Debugger) promptForPermission() (bool, error) {
	if !d.interactive {
		// No user to prompt. Reaching here means the caller already decided to debug (e.g. an
		// explicit `defang debug`), so proceed without asking.
		return true, nil
	}
	var aiDebug bool
	err := d.surveyor.AskOne(&survey.Confirm{
		Message: "Would you like to debug this with the Defang AI Agent?",
		// Default to Yes for everyone; the server selects an appropriate model per account, so
		// there is no need to gate the prompt client-side.
		Default: true,
		Help:    "This will send logs and artifacts to our backend and attempt to diagnose the issue and provide a solution.",
	}, &aiDebug, survey.WithStdio(term.DefaultTerm.Stdio()))
	if err != nil {
		return false, err
	}

	return aiDebug, err
}

func (d *Debugger) promptForFeedback() (string, error) {
	var feedback string
	err := d.surveyor.AskOne(&survey.Input{
		Message: "Was the debugging helpful?",
		Help:    "Your answer is sent to Defang to help us improve the debugging experience.",
	}, &feedback, survey.WithStdio(term.DefaultTerm.Stdio()))
	if err != nil {
		return "", err
	}

	return feedback, err
}

func buildDeploymentDebugPrompt(debugConfig DebugConfig) string {
	prompt := operationDescription(debugConfig.Operation)
	if debugConfig.ProviderID == nil {
		prompt += " with Defang."
	} else {
		prompt += fmt.Sprintf(" to %s with Defang.", debugConfig.ProviderID.Name())
	}

	prompt += " Help troubleshoot and recommend a solution. Look at the logs to understand what happened."

	// A teardown that fails is the moment resources get orphaned, and the agent has a tool for
	// exactly that. Without this the model reaches for the deployment-shaped remedies instead.
	if debugConfig.Operation != OperationDeploy {
		prompt += " A failed teardown usually leaves cloud resources behind," +
			" either because the deployment retains them or because another resource still references them," +
			" and those leftovers can exhaust a cloud quota later." +
			" Use the cleanup_resources tool to find them and remove what blocks the teardown," +
			" then run the destroy tool again so the deployment can finish removing the rest."
	}

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
		yaml, err := compose.MarshalYAML(debugConfig.Project)
		if err != nil {
			term.Println("Failed to marshal compose project to YAML for debug:", err)
		}
		prompt += fmt.Sprintf(
			"The compose files are at %s. The compose file is as follows:\n\n%s",
			debugConfig.Project.ComposeFiles,
			truncateHead(string(yaml), maxPromptComposeLen),
		)
	}
	return prompt
}

// operationDescription opens the prompt by naming what the user was actually doing, so the model
// does not read a teardown failure as a deployment failure.
func operationDescription(op DebugOperation) string {
	switch op {
	case OperationTeardown:
		return "An error occurred while tearing down this project"
	case OperationCleanup:
		return "An error occurred while cleaning up the cloud resources left behind by a teardown of this project"
	default:
		return "An error occurred while deploying this project"
	}
}

// The initial prompt must always fit in the model context, no matter how big the project or the
// deployment failure: cap each payload and let the agent page through the rest with its tools
// (e.g. the logs tool fetches 100 lines per call).
const (
	maxPromptErrorLen   = 2048
	maxPromptComposeLen = 8192
	// maxFeedbackLen caps the free-form survey answer that is sent to analytics.
	maxFeedbackLen = 1024
)

// truncateHead keeps the first max bytes of s; the start of a compose file (project and service
// definitions) is the most useful part. The cut is pulled back to a rune boundary, so a multibyte
// character straddling the limit is dropped whole rather than sent to the model as U+FFFD.
func truncateHead(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:runeBoundaryBefore(s, maxLen)] + "\n... (truncated; ask the user or use your tools for the rest)"
}

// truncateTail keeps the last max bytes of s; the end of an error is where the root cause lands.
// Like truncateHead, it cuts on a rune boundary, dropping at most three extra bytes.
func truncateTail(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return "(truncated) ..." + s[runeBoundaryAfter(s, len(s)-maxLen):]
}

// runeBoundaryBefore returns the largest index <= i that starts a rune, so s[:i] never ends
// mid-character. A UTF-8 rune is at most 4 bytes, so this backs up at most 3.
func runeBoundaryBefore(s string, i int) int {
	for i > 0 && !utf8.RuneStart(s[i]) {
		i--
	}
	return i
}

// runeBoundaryAfter returns the smallest index >= i that starts a rune, so s[i:] never begins
// mid-character. Moving forward (rather than back) keeps the result within maxLen bytes.
func runeBoundaryAfter(s string, i int) int {
	for i < len(s) && !utf8.RuneStart(s[i]) {
		i++
	}
	return i
}
