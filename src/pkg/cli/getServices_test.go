package cli

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/DefangLabs/defang/src/protos/io/defang/v1/defangv1connect"
	"github.com/bufbuild/connect-go"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var mockCreatedAt, _ = time.Parse(time.RFC3339, "2021-09-01T12:34:56Z")
var mockExpiresAt, _ = time.Parse(time.RFC3339, "2021-09-02T12:34:56Z")

type mockGetServicesHandler struct {
	defangv1connect.UnimplementedFabricControllerHandler
}

func (g *mockGetServicesHandler) GetServices(ctx context.Context, req *connect.Request[defangv1.GetServicesRequest]) (*connect.Response[defangv1.GetServicesResponse], error) {
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
				Endpoints:   []string{},
				Project:     "test",
				Etag:        "a1b2c3",
				Status:      "UNKNOWN",
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

func TestGetServices(t *testing.T) {
	ctx := context.Background()

	fabricServer := &mockGetServicesHandler{}
	_, handler := defangv1connect.NewFabricControllerHandler(fabricServer)
	server := httptest.NewServer(handler)
	t.Cleanup(func() {
		server.Close()
	})

	url := strings.TrimPrefix(server.URL, "http://")
	grpcClient := Connect(ctx, url)
	provider := cliClient.PlaygroundProvider{FabricClient: grpcClient}

	t.Run("no services", func(t *testing.T) {
		err := GetServices(ctx, "empty", &provider, false)
		var expectedError ErrNoServices
		if !errors.As(err, &expectedError) {
			t.Fatalf("expected GetServices() error to be of type ErrNoServices, got: %v", err)
		}
	})

	t.Run("some services", func(t *testing.T) {
		stdout, _ := term.SetupTestTerm(t)

		err := GetServices(ctx, "test", &provider, false)
		if err != nil {
			t.Fatalf("GetServices() error = %v", err)
		}
		expectedOutput := "\x1b[1m\nService  Deployment  PublicFqdn                 PrivateFqdn  Status   \x1b[0m" + `
foo      a1b2c3      test-foo.prod1.defang.dev               UNKNOWN
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
		err := GetServices(ctx, "empty", &provider, false)
		var expectedError ErrNoServices
		if !errors.As(err, &expectedError) {
			t.Fatalf("expected GetServices() error to be of type ErrNoServices, got: %v", err)
		}
	})

	t.Run("some services long", func(t *testing.T) {
		stdout, _ := term.SetupTestTerm(t)

		err := GetServices(ctx, "test", &provider, true)
		if err != nil {
			t.Fatalf("GetServices() error = %v", err)
		}
		expectedOutput := "expiresAt: \"2021-09-02T12:34:56Z\"\n" +
			"project: test\n" +
			"services:\n" +
			"    - createdAt: \"2021-09-01T12:34:56Z\"\n" +
			"      etag: a1b2c3\n" +
			"      project: test\n" +
			"      publicFqdn: test-foo.prod1.defang.dev\n" +
			"      service:\n" +
			"        name: foo\n" +
			"      status: UNKNOWN\n\n"

		receivedLines := stdout.String()
		expectedLines := expectedOutput

		if receivedLines != expectedLines {
			t.Errorf("expected %q to equal %q", receivedLines, expectedLines)
		}
	})
}
