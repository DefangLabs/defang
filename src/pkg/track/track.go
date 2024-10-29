package track

import (
	"strings"
	"sync"

	"github.com/DefangLabs/defang/src/pkg"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var disableAnalytics = pkg.GetenvBool("DEFANG_DISABLE_ANALYTICS")

func P(name string, value interface{}) cliClient.Property {
	return cliClient.Property{Name: name, Value: value}
}

var Fabric cliClient.FabricClient

// trackWG is used to wait for all tracking to complete.
var trackWG = sync.WaitGroup{}

// Track sends a tracking event to the server in a separate goroutine.
func Evt(name string, props ...cliClient.Property) {
	if disableAnalytics {
		return
	}
	trackWG.Add(1)
	go func(fabric cliClient.FabricClient) {
		defer trackWG.Done()
		if fabric == nil {
			term.Debugf("No FabricClient to track event: %v", name)
			return
		}
		fabric.Track(name, props...)
	}(Fabric)
}

// FlushAllTracking waits for all tracking goroutines to complete.
func FlushAllTracking() {
	trackWG.Wait()
}

func isCompletionCommand(cmd *cobra.Command) bool {
	return cmd.Name() == cobra.ShellCompRequestCmd || (cmd.Parent() != nil && cmd.Parent().Name() == "completion")
}

// trackCmd sends a tracking event for a Cobra command and its arguments.
func Cmd(cmd *cobra.Command, verb string, props ...cliClient.Property) {
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
