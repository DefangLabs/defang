package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

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
					},
					Status:     "UNKNOWN",
					Domainname: "example.com",
					Endpoints:  []string{},
				},
			},
			expectedLines: []string{
				"SERVICE   DEPLOYMENT  STATE          FQDN  ENDPOINT             STATUS",
				"service1              NOT_SPECIFIED        https://example.com  UNKNOWN",
				"",
			},
		},
		{
			name: "Service with Domainname",
			serviceinfos: []*defangv1.ServiceInfo{
				{
					Service: &defangv1.Service{
						Name: "service1",
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
				"SERVICE   DEPLOYMENT  STATE          FQDN  ENDPOINT             STATUS",
				"service1              NOT_SPECIFIED        https://example.com  UNKNOWN",
				"",
			},
		},
		{
			name: "endpoint without port",
			serviceinfos: []*defangv1.ServiceInfo{
				{
					Service: &defangv1.Service{
						Name: "service1",
					},
					Status: "UNKNOWN",
					Endpoints: []string{
						"service1",
					},
				},
			},
			expectedLines: []string{
				"SERVICE   DEPLOYMENT  STATE          FQDN  ENDPOINT  STATUS",
				"service1              NOT_SPECIFIED        N/A       UNKNOWN",
				"",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset stdout before each test
			stdout.Reset()

			_ = PrintServiceStatesAndEndpoints(tt.serviceinfos)
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
