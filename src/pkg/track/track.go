package track

import (
	"fmt"
	"strings"
	"sync"

	"github.com/DefangLabs/defang/src/pkg"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const maxPropertyCharacterLength = 255 // chars per property in tracking event

var disableAnalytics = pkg.GetenvBool("DEFANG_DISABLE_ANALYTICS")
var logPropertyNamePrefix = "logs"

type Property = cliClient.Property

// P creates a Property with the given name and value.
func P(name string, value any) Property {
	return Property{Name: name, Value: value}
}

var Tracker interface {
	Track(string, ...Property) error
}

// trackWG is used to wait for all asynchronous tracking to complete.
var trackWG = sync.WaitGroup{}

// Evt sends a tracking event to the server in a separate goroutine.
// This function can take in optional key-value pairs which is a type called Property.
//
// Example:
//
//	client := "vscode"
//	track.Evt("MCP Setup Client:", track.P("client", client))
func Evt(name string, props ...Property) {
	if disableAnalytics {
		return
	}
	tracker := Tracker
	if tracker == nil {
		term.Debugf("untracked event %q: %v", name, props)
		return
	}

	// filter out props with a name prefix of logPropertyNamePrefix, they should already be in the debug output
	var filteredProps []Property
	for _, p := range props {
		if strings.HasPrefix(p.Name, logPropertyNamePrefix) {
			continue
		}
		filteredProps = append(filteredProps, p)
	}

	term.Debugf("tracking event %q: %v", name, filteredProps)
	trackWG.Add(1)
	go func() {
		defer trackWG.Done()
		tracker.Track(name, props...)
	}()
}

// FlushAllTracking waits for all tracking goroutines to complete.
func FlushAllTracking() {
	trackWG.Wait()
}

// function to break a set of messages into smaller chunks for tracking
// There is a set size limit per property for tracking
func MakeEventLogProperties(name string, message []string) []Property {
	var trackMsg []Property

	for i, msg := range message {
		if len(msg) > maxPropertyCharacterLength {
			msg = msg[:maxPropertyCharacterLength]
		}
		propName := fmt.Sprintf("%s-%d", name, i+1)
		trackMsg = append(trackMsg, P(propName, msg))
	}
	return trackMsg
}

func EvtWithTerm(eventName string, extraProps ...Property) {
	messages := term.DefaultTerm.GetAllMessages()
	logProps := MakeEventLogProperties(logPropertyNamePrefix, messages)
	allProps := append(extraProps, logProps...)
	Evt(eventName, allProps...)
}

func isCompletionCommand(cmd *cobra.Command) bool {
	return cmd.Name() == cobra.ShellCompRequestCmd || (cmd.Parent() != nil && cmd.Parent().Name() == "completion")
}

// Cmd sends a tracking event for a Cobra command and its arguments.
func Cmd(cmd *cobra.Command, verb string, props ...Property) {
	command := "Implicit"
	if cmd != nil {
		// Ignore tracking for shell completion requests
		if isCompletionCommand(cmd) {
			return
		}
		command = cmd.Name()
		calledAs := cmd.CalledAs()
		cmd.VisitParents(func(c *cobra.Command) {
			calledAs = c.CalledAs() + " " + calledAs
			if c.HasParent() { // skip root command
				command = c.Name() + "-" + command
			}
		})

		props = append(props,
			P("CalledAs", calledAs),
			P("version", cmd.Root().Version),
		)

		cmd.Flags().Visit(func(f *pflag.Flag) {
			props = append(props, P(f.Name, f.Value))
		})
	}
	Evt(strings.ToTitle(command+" "+verb), props...)
}
