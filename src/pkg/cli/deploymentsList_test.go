package cli

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/DefangLabs/defang/src/protos/io/defang/v1/defangv1connect"
	connect "github.com/bufbuild/connect-go"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type mockListDeploymentsHandler struct {
	defangv1connect.UnimplementedFabricControllerHandler
}

func (mockListDeploymentsHandler) WhoAmI(context.Context, *connect.Request[emptypb.Empty]) (*connect.Response[defangv1.WhoAmIResponse], error) {
	return connect.NewResponse(&defangv1.WhoAmIResponse{}), nil
}

func (mockListDeploymentsHandler) ListDeployments(ctx context.Context, req *connect.Request[defangv1.ListDeploymentsRequest]) (*connect.Response[defangv1.ListDeploymentsResponse], error) {
	var deployments []*defangv1.Deployment

	if req.Msg.Project == "empty" {
		deployments = []*defangv1.Deployment{}
	} else {
		deployments = []*defangv1.Deployment{
			{
				Id:                "a1b2c3",
				Project:           "test",
				Provider:          defangv1.Provider_DEFANG,
				ProviderAccountId: "1234567890",
				ProviderString:    "playground",
				Region:            "us-test-2",
				Timestamp:         timestamppb.New(time.Time{}),
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
	t.Cleanup(server.Close)

	url := strings.TrimPrefix(server.URL, "http://")
	grpcClient, _ := Connect(ctx, url)

	t.Run("no deployments", func(t *testing.T) {
		stdout, _ := term.SetupTestTerm(t)
		err := DeploymentsList(ctx, defangv1.DeploymentType_DEPLOYMENT_TYPE_HISTORY, "empty", grpcClient, 10)
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
		err := DeploymentsList(ctx, defangv1.DeploymentType_DEPLOYMENT_TYPE_HISTORY, "test", grpcClient, 10)
		if err != nil {
			t.Fatalf("DeploymentsList() error = %v", err)
		}
		expectedOutput := "\x1b[1m\nPROJECTNAME  PROVIDER  ACCOUNTID   REGION     DEPLOYMENT  DEPLOYEDAT\x1b[0m" + `
test         defang    1234567890  us-test-2  a1b2c3      ` + time.Time{}.Local().Format(time.RFC3339) + "\n"

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
