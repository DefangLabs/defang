package command

import (
	"bytes"
	"os"
	"strings"
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

	_ = printServiceStatesAndEndpoints([]*defangv1.ServiceInfo{
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
	const expectedOutput = `Id  Name      Status         Endpoints
    service1  NOT_SPECIFIED  example.com, service1.internal
`
	receivedLines := strings.Split(stdout.String(), "\n")
	expectedLines := strings.Split(expectedOutput, "\n")

	if len(receivedLines) != len(expectedLines) {
		t.Errorf("Expected %v lines, received %v", len(expectedLines), len(receivedLines))
	}

	for i, receivedLine := range receivedLines {
		receivedLine = strings.TrimRight(receivedLine, " ")
		if receivedLine != expectedLines[i] {
			t.Errorf("\n-%v\n+%v", expectedLines[i], receivedLine)
		}
	}
}

func TestPrintServiceStatesAndEndpointsAndDomainname(t *testing.T) {
	defaultTerm := term.DefaultTerm
	t.Cleanup(func() {
		term.DefaultTerm = defaultTerm
	})

	var stdout, stderr bytes.Buffer
	term.DefaultTerm = term.NewTerm(os.Stdin, &stdout, &stderr)

	_ = printServiceStatesAndEndpoints([]*defangv1.ServiceInfo{
		{
			Service: &defangv1.Service{
				Name: "service1",
				Ports: []*defangv1.Port{
					{Mode: defangv1.Mode_INGRESS},
					{Mode: defangv1.Mode_HOST},
				},
			},
			Status:     "UNKNOWN",
			Domainname: "example.com",
			Endpoints: []string{
				"example.com",
				"service1.internal",
			},
		}})
	expectedLines := []string{
		"Id  Name      Status         Endpoints                       DomainName",
		"    service1  NOT_SPECIFIED  example.com, service1.internal  https://example.com",
		" * Run `defang cert generate` to get a TLS certificate for your service(s)",
		"",
	}
	receivedLines := strings.Split(stdout.String(), "\n")

	if len(receivedLines) != len(expectedLines) {
		t.Errorf("Expected %v lines, received %v", len(expectedLines), len(receivedLines))
	}

	for i, receivedLine := range receivedLines {
		receivedLine = strings.TrimRight(receivedLine, " ")
		if receivedLine != expectedLines[i] {
			t.Errorf("\n-%v\n+%v", expectedLines[i], receivedLine)
		}
	}
}
