package command

import (
	"bytes"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func TestPrintPlaygroundPortalServiceURLs(t *testing.T) {
	defaultTerm := term.DefaultTerm
	t.Cleanup(func() {
		term.DefaultTerm = defaultTerm
	})

	var stdout, stderr bytes.Buffer
	term.DefaultTerm = term.NewTerm(&stdout, &stderr)

	provider = cliClient.ProviderDefang
	cluster = cli.DefaultCluster
	printPlaygroundPortalServiceURLs([]*defangv1.ServiceInfo{
		{
			Service: &defangv1.ServiceID{Name: "service1"},
		}})
	const want = ` * Monitor your services' status in the defang portal
   - https://portal.defang.dev/service/service1
`
	if got := stdout.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPrintEndpoints(t *testing.T) {
	defaultTerm := term.DefaultTerm
	t.Cleanup(func() {
		term.DefaultTerm = defaultTerm
	})

	var stdout, stderr bytes.Buffer
	term.DefaultTerm = term.NewTerm(&stdout, &stderr)

	printEndpoints([]*defangv1.ServiceInfo{
		{
			Service: &defangv1.ServiceID{Name: "service1"},
			Status:  "UNKNOWN",
			Endpoints: []string{
				"example.com:443",
				"service1.internal",
			},
		}})
	const want = ` * Service service1 is in state UNKNOWN and will be available at:
   - https://example.com
   - service1.internal
`
	if got := stdout.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
