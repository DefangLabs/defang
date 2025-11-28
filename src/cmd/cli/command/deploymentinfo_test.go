package command

import (
	"bytes"
	"os"
	"strings"
	"testing"

	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	pcluster "github.com/DefangLabs/defang/src/pkg/cluster"
	"github.com/DefangLabs/defang/src/pkg/globals"
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

	globals.Config.ProviderID = cliClient.ProviderDefang
	globals.Config.Cluster = pcluster.DefaultCluster
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

func TestPrintServiceStatesAndEndpointsAndDomainname(t *testing.T) {
	defaultTerm := term.DefaultTerm
	t.Cleanup(func() {
		term.DefaultTerm = defaultTerm
	})

	var stdout, stderr bytes.Buffer
	term.DefaultTerm = term.NewTerm(os.Stdin, &stdout, &stderr)

	tests := []struct {
		name          string
		serviceinfos  []*defangv1.ServiceInfo
		expectedLines []string
	}{
		{
			name: "empty endpoint list",
			serviceinfos: []*defangv1.ServiceInfo{
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
					Endpoints:  []string{},
				},
			},
			expectedLines: []string{
				"DEPLOYMENT  NAME      STATUS         ENDPOINTS  DOMAINNAME",
				"            service1  NOT_SPECIFIED  N/A        https://example.com",
				" * Run `defang cert generate` to get a TLS certificate for your service(s)",
				"",
			},
		},
		{
			name: "Service with Domainname",
			serviceinfos: []*defangv1.ServiceInfo{
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
						"service1.internal:80",
					},
				},
			},
			expectedLines: []string{
				"DEPLOYMENT  NAME      STATUS         ENDPOINTS                                  DOMAINNAME",
				"            service1  NOT_SPECIFIED  https://example.com, service1.internal:80  https://example.com",
				" * Run `defang cert generate` to get a TLS certificate for your service(s)",
				"",
			},
		},
		{
			name: "endpoint without port",
			serviceinfos: []*defangv1.ServiceInfo{
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
						"service1",
					},
				},
			},
			expectedLines: []string{
				"DEPLOYMENT  NAME      STATUS         ENDPOINTS",
				"            service1  NOT_SPECIFIED  https://service1",
				"",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset stdout before each test
			stdout.Reset()

			_ = printServiceStatesAndEndpoints(tt.serviceinfos)
			receivedLines := strings.Split(stdout.String(), "\n")

			if len(receivedLines) != len(tt.expectedLines) {
				t.Errorf("Expected %v lines, received %v", len(tt.expectedLines), len(receivedLines))
			}

			for i, receivedLine := range receivedLines {
				receivedLine = strings.TrimRight(receivedLine, " ")
				if receivedLine != tt.expectedLines[i] {
					t.Errorf("\n-%v\n+%v", tt.expectedLines[i], receivedLine)
				}
			}
		})
	}
}
