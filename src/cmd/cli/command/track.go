package command

import (
	"strings"
	"sync"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
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
	trackWG.Add(1)
	go func(client cliClient.FabricClient) {
		if client == nil {
			client = cli.Connect(cluster, nil)
		}
		defer trackWG.Done()
		_ = client.Track(name, props...)
	}(client)
}

// FlushAllTracking waits for all tracking goroutines to complete.
func FlushAllTracking() {
	trackWG.Wait()
}

func IsCompletionCommand(cmd *cobra.Command) bool {
	return cmd.Name() == cobra.ShellCompRequestCmd || (cmd.Parent() != nil && cmd.Parent().Name() == "completion")
}

// trackCmd sends a tracking event for a Cobra command and its arguments.
func trackCmd(cmd *cobra.Command, verb string, props ...P) {
	command := "Implicit"
	if cmd != nil {
		// Ignore tracking for shell completion requests
		if IsCompletionCommand(cmd) {
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
			P{Name: "CalledAs", Value: calledAs},
			P{Name: "version", Value: cmd.Root().Version},
		)
		cmd.Flags().Visit(func(f *pflag.Flag) {
			props = append(props, P{Name: f.Name, Value: f.Value})
		})
	}
	Track(strings.Title(command+" "+verb), props...)
}
