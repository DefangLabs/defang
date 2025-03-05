package command

import (
	"bytes"
	"os"
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
	term.DefaultTerm = term.NewTerm(os.Stdin, &stdout, &stderr)

	providerID = cliClient.ProviderDefang
	cluster = cli.DefaultCluster
	printPlaygroundPortalServiceURLs([]*defangv1.ServiceInfo{
		{
			Service: &defangv1.Service{Name: "service1"},
		}})
	const want = ` * Monitor your services' status in the defang portal
   - https://portal.defang.io/service/service1
`
	if got := stdout.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPrintServiceStatesAndEndpoints(t *testing.T) {
	defaultTerm := term.DefaultTerm
	t.Cleanup(func() {
		term.DefaultTerm = defaultTerm
	})

	var stdout, stderr bytes.Buffer
	term.DefaultTerm = term.NewTerm(os.Stdin, &stdout, &stderr)

	printServiceStatesAndEndpoints([]*defangv1.ServiceInfo{
		{
			Service: &defangv1.Service{
				Name: "service1",
				Ports: []*defangv1.Port{
					{Mode: defangv1.Mode_INGRESS},
					{Mode: defangv1.Mode_HOST},
				},
			},
			Status: "UNKNOWN",
			Endpoints: []string{
				"example.com",
				"service1.internal",
			},
		}})
	const want = ` * Service service1 has status UNKNOWN and will be available at:
   - https://example.com
   - service1.internal
`
	if got := stdout.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
