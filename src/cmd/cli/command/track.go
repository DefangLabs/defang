package command

import (
	"strings"
	"sync"

	"github.com/defang-io/defang/src/pkg"
	"github.com/defang-io/defang/src/pkg/cli"
	cliClient "github.com/defang-io/defang/src/pkg/cli/client"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var disableAnalytics = pkg.GetenvBool("DEFANG_DISABLE_ANALYTICS")

type P = cliClient.Property // shorthand for tracking properties

// trackWG is used to wait for all tracking to complete.
var trackWG = sync.WaitGroup{}

// Track sends a tracking event to the server in a separate goroutine.
func Track(name string, props ...P) {
	if disableAnalytics {
		return
	}
	if client == nil {
		client, _ = cli.Connect(cluster)
	}
	trackWG.Add(1)
	go func(client cliClient.Client) {
		defer trackWG.Done()
		_ = client.Track(name, props...)
	}(client)
}

// flushAllTracking waits for all tracking goroutines to complete.
func FlushAllTracking() {
	trackWG.Wait()
}

// trackCmd sends a tracking event for a Cobra command and its arguments.
func trackCmd(cmd *cobra.Command, verb string, props ...P) {
	command := "Unknown"
	if cmd != nil {
		command = cmd.Name()
		// Ignore tracking for shell completion requests
		if command == cobra.ShellCompRequestCmd {
			return
		}
		calledAs := cmd.CalledAs()
		cmd.VisitParents(func(c *cobra.Command) {
			calledAs = c.CalledAs() + " " + calledAs
			if c.HasParent() { // skip root command
				command = c.Name() + "-" + command
			}
		})
		props = append(props, P{Name: "CalledAs", Value: calledAs})
		cmd.Flags().Visit(func(f *pflag.Flag) {
			props = append(props, P{Name: f.Name, Value: f.Value})
		})
	}
	Track(strings.Title(command+" "+verb), props...)
}
