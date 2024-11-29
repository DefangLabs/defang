package cli

import (
	"bytes"
	"context"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/DefangLabs/defang/src/protos/io/defang/v1/defangv1connect"
	"github.com/bufbuild/connect-go"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type grpcListDeploymentsMockHandler struct {
	defangv1connect.UnimplementedFabricControllerHandler
}

func (g *grpcListDeploymentsMockHandler) ListDeployments(context.Context, *connect.Request[defangv1.ListDeploymentsRequest]) (*connect.Response[defangv1.ListDeploymentsResponse], error) {
	return connect.NewResponse(&defangv1.ListDeploymentsResponse{
		Deployments: []*defangv1.Deployment{
			{
				Id:                "a1b2c3",
				Project:           "test",
				Provider:          "playground",
				ProviderAccountId: "1234567890",
				Timestamp:         timestamppb.Now(),
			},
		},
	}), nil
}

func TestDeploymentsList(t *testing.T) {
	ctx := context.Background()

	var stdout, stderr bytes.Buffer
	testTerm := term.NewTerm(&stdout, io.MultiWriter(&stderr))
	testTerm.ForceColor(true)
	defaultTerm := term.DefaultTerm
	term.DefaultTerm = testTerm
	t.Cleanup(func() {
		term.DefaultTerm = defaultTerm
	})

	loader := client.MockLoader{Project: &compose.Project{Name: "test"}}
	fabricServer := &grpcListDeploymentsMockHandler{}
	_, handler := defangv1connect.NewFabricControllerHandler(fabricServer)

	server := httptest.NewServer(handler)
	defer server.Close()
	url := strings.TrimPrefix(server.URL, "http://")
	client := NewGrpcClient(ctx, url)
	err := DeploymentsList(ctx, loader, client)
	if err != nil {
		t.Fatalf("DeploymentsList() error = %v", err)
	}

	expectedOutput := `Id      Provider    DeployedAt
a1b2c3  playground  ` + timestamppb.Now().AsTime().Format("2006-01-02T15:04:05Z07:00") + `
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
