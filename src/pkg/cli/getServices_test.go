package cli

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/DefangLabs/defang/src/protos/io/defang/v1/defangv1connect"
	"github.com/bufbuild/connect-go"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var mockCreatedAt = timestamppb.Now()
var mockExpiresAt = timestamppb.New(time.Now().Add(1 * time.Hour))

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
				CreatedAt:   mockCreatedAt,
			},
		}
	}

	return connect.NewResponse(&defangv1.GetServicesResponse{
		Project:   project,
		Services:  services,
		ExpiresAt: mockExpiresAt,
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
	grpcClient := NewGrpcClient(ctx, url)
	provider := cliClient.PlaygroundProvider{GrpcClient: grpcClient}

	t.Run("no services", func(t *testing.T) {
		stdout, _ := term.SetupTestTerm(t)
		loader := cliClient.MockLoader{Project: &compose.Project{Name: "empty"}}
		err := GetServices(ctx, loader, &provider, false)
		if err == nil {
			t.Fatalf("expected GetServices() error to not be nil")
		}

		receivedOutput := stdout.String()
		expectedOutput := "no services found in project \"empty\""

		if !strings.Contains(stdout.String(), expectedOutput) {
			t.Errorf("Expected %s to contain %s", receivedOutput, expectedOutput)
		}
	})

	t.Run("some services", func(t *testing.T) {
		stdout, _ := term.SetupTestTerm(t)
		loader := cliClient.MockLoader{Project: &compose.Project{Name: "test"}}

		err := GetServices(ctx, loader, &provider, false)
		if err != nil {
			t.Fatalf("GetServices() error = %v", err)
		}
		expectedOutput := `Name  Etag    PublicFqdn                 PrivateFqdn  Status
foo   a1b2c3  test-foo.prod1.defang.dev               UNKNOWN
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
		stdout, _ := term.SetupTestTerm(t)
		loader := cliClient.MockLoader{Project: &compose.Project{Name: "empty"}}
		err := GetServices(ctx, loader, &provider, false)
		if err == nil {
			t.Fatalf("expected GetServices() error to not be nil")
		}

		receivedOutput := stdout.String()
		expectedOutput := "no services found in project \"empty\""

		if !strings.Contains(stdout.String(), expectedOutput) {
			t.Errorf("Expected %s to contain %s", receivedOutput, expectedOutput)
		}
	})

	t.Run("some services long", func(t *testing.T) {
		stdout, _ := term.SetupTestTerm(t)
		loader := cliClient.MockLoader{Project: &compose.Project{Name: "test"}}

		err := GetServices(ctx, loader, &provider, true)
		if err != nil {
			t.Fatalf("GetServices() error = %v", err)
		}
		expectedOutput := "expiresAt: \"" + mockExpiresAt.AsTime().Format(time.RFC3339) + `"
project: test
services:
    - createdAt: "` + mockCreatedAt.AsTime().Format(time.RFC3339) + `"
      etag: a1b2c3
      project: test
      publicFqdn: test-foo.prod1.defang.dev
      service:
        name: foo
      status: UNKNOWN

`

		receivedLines := stdout.String()
		expectedLines := expectedOutput

		if receivedLines != expectedLines {
			t.Errorf("expected %q to equal %q", receivedLines, expectedLines)
		}
	})
}
