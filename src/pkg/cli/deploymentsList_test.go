package cli

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/DefangLabs/defang/src/protos/io/defang/v1/defangv1connect"
	connect "github.com/bufbuild/connect-go"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type mockListDeploymentsHandler struct {
	defangv1connect.UnimplementedFabricControllerHandler
}

func (g *mockListDeploymentsHandler) ListDeployments(ctx context.Context, req *connect.Request[defangv1.ListDeploymentsRequest]) (*connect.Response[defangv1.ListDeploymentsResponse], error) {
	var deployments []*defangv1.Deployment

	if req.Msg.Project == "empty" {
		deployments = []*defangv1.Deployment{}
	} else {
		deployments = []*defangv1.Deployment{
			{
				Id:                "a1b2c3",
				Project:           "test",
				Provider:          "playground",
				ProviderAccountId: "1234567890",
				Timestamp:         timestamppb.Now(),
			},
		}
	}

	return connect.NewResponse(&defangv1.ListDeploymentsResponse{
		Deployments: deployments,
	}), nil
}

func TestDeploymentsList(t *testing.T) {
	ctx := context.Background()

	fabricServer := &mockListDeploymentsHandler{}
	_, handler := defangv1connect.NewFabricControllerHandler(fabricServer)
	server := httptest.NewServer(handler)
	t.Cleanup(func() {
		server.Close()
	})

	url := strings.TrimPrefix(server.URL, "http://")
	client := NewGrpcClient(ctx, url)

	t.Run("no deployments", func(t *testing.T) {
		stdout, _ := term.SetupTestTerm(t)
		err := DeploymentsList(ctx, "empty", client)
		if err != nil {
			t.Fatalf("DeploymentsList() error = %v", err)
		}

		receivedOutput := stdout.String()
		expectedOutput := "No deployments"

		if !strings.Contains(stdout.String(), expectedOutput) {
			t.Errorf("Expected %s to contain %s", receivedOutput, expectedOutput)
		}
	})

	t.Run("some deployments", func(t *testing.T) {
		stdout, _ := term.SetupTestTerm(t)
		err := DeploymentsList(ctx, "test", client)
		if err != nil {
			t.Fatalf("DeploymentsList() error = %v", err)
		}
		expectedOutput := "\x1b[1m\nDeployment  Provider    DeployedAt            \x1b[0m" + `
a1b2c3      playground  ` + timestamppb.Now().AsTime().Format("2006-01-02T15:04:05Z07:00") + `
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
}
