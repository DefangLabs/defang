package cli

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/DefangLabs/defang/src/protos/io/defang/v1/defangv1connect"
	"github.com/bufbuild/connect-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var mockCreatedAt, _ = time.Parse(time.RFC3339, "2021-09-01T12:34:56Z")
var mockExpiresAt, _ = time.Parse(time.RFC3339, "2021-09-02T12:34:56Z")

type mockGetServicesHandler struct {
	defangv1connect.UnimplementedFabricControllerHandler
}

func (mockGetServicesHandler) WhoAmI(context.Context, *connect.Request[emptypb.Empty]) (*connect.Response[defangv1.WhoAmIResponse], error) {
	return connect.NewResponse(&defangv1.WhoAmIResponse{}), nil
}

func (mockGetServicesHandler) GetServices(ctx context.Context, req *connect.Request[defangv1.GetServicesRequest]) (*connect.Response[defangv1.GetServicesResponse], error) {
	project := req.Msg.Project
	var services []*defangv1.ServiceInfo

	if project == "empty" {
		services = []*defangv1.ServiceInfo{}
	} else {
		services = []*defangv1.ServiceInfo{
			{
				Service: &defangv1.Service{
					Name: "foo",
				},
				Endpoints:   []string{"test-foo--3000.prod1.defang.dev"},
				Project:     "test",
				Etag:        "a1b2c3",
				Status:      "NOT_SPECIFIED",
				PublicFqdn:  "test-foo.prod1.defang.dev",
				PrivateFqdn: "",
				CreatedAt:   timestamppb.New(mockCreatedAt),
			},
		}
	}

	return connect.NewResponse(&defangv1.GetServicesResponse{
		Project:   project,
		Services:  services,
		ExpiresAt: timestamppb.New(mockExpiresAt),
	}), nil
}

func TestPrintServices(t *testing.T) {
	ctx := t.Context()

	fabricServer := &mockGetServicesHandler{}
	_, handler := defangv1connect.NewFabricControllerHandler(fabricServer)
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	url := strings.TrimPrefix(server.URL, "http://")
	grpcClient := Connect(url, types.TenantUnset)
	provider := client.PlaygroundProvider{FabricClient: grpcClient}

	t.Run("no services", func(t *testing.T) {
		err := PrintServices(ctx, "empty", &provider)
		var expectedError ErrNoServices
		if !errors.As(err, &expectedError) {
			t.Fatalf("expected PrintServices error to be of type ErrNoServices, got: %v", err)
		}
	})

	t.Run("some services", func(t *testing.T) {
		stdout, _ := term.SetupTestTerm(t)

		err := PrintServices(ctx, "test", &provider)
		if err != nil {
			t.Fatalf("PrintServices error = %v", err)
		}
		expectedOutput := "\x1b[1m\nSERVICE  DEPLOYMENT  STATE          FQDN                       ENDPOINT                           HEALTHCHECKSTATUS\x1b[0m" + `
foo      a1b2c3      NOT_SPECIFIED  test-foo.prod1.defang.dev  https://test-foo.prod1.defang.dev  unhealthy (404 Not Found)
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
	})

	t.Run("no services long", func(t *testing.T) {
		err := PrintServices(ctx, "empty", &provider)
		var expectedError ErrNoServices
		if !errors.As(err, &expectedError) {
			t.Fatalf("expected PrintServices error to be of type ErrNoServices, got: %v", err)
		}
	})

	t.Run("some services long", func(t *testing.T) {
		stdout, _ := term.SetupTestTerm(t)

		err := PrintLongServices(ctx, "test", &provider)
		if err != nil {
			t.Fatalf("PrintServices error = %v", err)
		}
		expectedOutput := "expiresAt: \"2021-09-02T12:34:56Z\"\n" +
			"project: test\n" +
			"services:\n" +
			"    - createdAt: \"2021-09-01T12:34:56Z\"\n" +
			"      endpoints:\n" +
			"        - test-foo--3000.prod1.defang.dev\n" +
			"      etag: a1b2c3\n" +
			"      project: test\n" +
			"      publicFqdn: test-foo.prod1.defang.dev\n" +
			"      service:\n" +
			"        name: foo\n" +
			"      status: NOT_SPECIFIED\n\n"

		receivedLines := stdout.String()
		expectedLines := expectedOutput

		if receivedLines != expectedLines {
			t.Errorf("expected %q to equal %q", receivedLines, expectedLines)
		}
	})
}

func TestGetServiceStatesAndEndpoints(t *testing.T) {
	tests := []struct {
		name             string
		serviceinfos     []*defangv1.ServiceInfo
		expectedServices []ServiceLineItem
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
			expectedServices: []ServiceLineItem{
				{
					Service:  "service1",
					Status:   "UNKNOWN",
					Endpoint: "https://example.com",
				},
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
			expectedServices: []ServiceLineItem{
				{
					Service:  "service1",
					Status:   "UNKNOWN",
					Endpoint: "https://example.com",
				},
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
			expectedServices: []ServiceLineItem{
				{
					Service:  "service1",
					Status:   "UNKNOWN",
					Endpoint: "N/A",
				},
			},
		},
		{
			name: "with acme cert",
			serviceinfos: []*defangv1.ServiceInfo{
				{
					Service: &defangv1.Service{
						Name: "service1",
					},
					Status:      "UNKNOWN",
					UseAcmeCert: true,
					Endpoints: []string{
						"service1",
					},
				},
			},
			expectedServices: []ServiceLineItem{
				{
					Service:      "service1",
					Status:       "UNKNOWN",
					Endpoint:     "N/A",
					AcmeCertUsed: true,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			services, err := ServiceLineItemsFromServiceInfos(tt.serviceinfos)
			require.NoError(t, err)

			assert.Len(t, services, len(tt.expectedServices))
			for i, svc := range services {
				assert.Equal(t, tt.expectedServices[i].Service, svc.Service)
				assert.Equal(t, tt.expectedServices[i].Status, svc.Status)
				assert.Equal(t, tt.expectedServices[i].Endpoint, svc.Endpoint)
				assert.Equal(t, tt.expectedServices[i].AcmeCertUsed, svc.AcmeCertUsed)
			}
		})
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
		services      []ServiceLineItem
		expectedLines []string
	}{
		{
			name: "empty endpoint list",
			services: []ServiceLineItem{
				{
					Service:  "service1",
					Status:   "UNKNOWN",
					Endpoint: "https://example.com",
				},
			},
			expectedLines: []string{
				"SERVICE   DEPLOYMENT  STATE          FQDN  ENDPOINT",
				"service1              NOT_SPECIFIED        https://example.com",
				"",
			},
		},
		{
			name: "Service with Domainname",
			services: []ServiceLineItem{
				{
					Service:  "service1",
					Status:   "UNKNOWN",
					Endpoint: "https://example.com",
				},
			},
			expectedLines: []string{
				"SERVICE   DEPLOYMENT  STATE          FQDN  ENDPOINT",
				"service1              NOT_SPECIFIED        https://example.com",
				"",
			},
		},
		{
			name: "with acme cert",
			services: []ServiceLineItem{
				{
					Service:      "service1",
					Status:       "UNKNOWN",
					Endpoint:     "N/A",
					AcmeCertUsed: true,
				},
			},
			expectedLines: []string{
				"SERVICE   DEPLOYMENT  STATE          FQDN  ENDPOINT",
				"service1              NOT_SPECIFIED        N/A",
				" * Run `defang cert generate` to get a TLS certificate for your service(s)",
				"",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset stdout before each test
			stdout.Reset()

			err := PrintServiceStatesAndEndpoints(tt.services)
			require.NoError(t, err)
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

func TestRunHealthcheck(t *testing.T) {
	ctx := t.Context()

	// Start a test HTTP server
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthy":
			w.WriteHeader(http.StatusOK)
		case "/unhealthy":
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(testServer.Close)

	tests := []struct {
		name            string
		endpoint        string
		healthcheckPath string
		expectedStatus  string
	}{
		{
			name:            "Healthy service",
			endpoint:        testServer.URL,
			healthcheckPath: "/healthy",
			expectedStatus:  "healthy",
		},
		{
			name:            "Unhealthy service",
			endpoint:        testServer.URL,
			healthcheckPath: "/unhealthy",
			expectedStatus:  "unhealthy (500 Internal Server Error)",
		},
		{
			name:            "Service not found",
			endpoint:        testServer.URL,
			healthcheckPath: "/notfound",
			expectedStatus:  "unhealthy (404 Not Found)",
		},
		{
			name:            "Invalid endpoint",
			endpoint:        "http://invalid-endpoint-238hf83wfnrewanf.com",
			healthcheckPath: "/",
			expectedStatus:  "unknown (DNS error)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, err := RunHealthcheck(ctx, "test-service", tt.endpoint, tt.healthcheckPath)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, status)
		})
	}
}

func TestRunHealthcheckTLSError(t *testing.T) {
	ctx := t.Context()

	// Start a test HTTPS server with a self-signed certificate
	testServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(testServer.Close)

	status, err := RunHealthcheck(ctx, "test-service", testServer.URL, "/healthy")
	require.NoError(t, err)
	assert.Equal(t, "unknown (TLS certificate error)", status)
}
