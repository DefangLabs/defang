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
)

var emptyDeployments []*defangv1.Deployment
var activeDeployments = []*defangv1.Deployment{
	{Project: "projectAA", Provider: defangv1.Provider_AWS, Region: "us-east-1"},
	{Project: "projectAB", Provider: defangv1.Provider_AWS, Region: "us-east-2"},
	{Project: "projectAC", Provider: defangv1.Provider_AWS, Region: "us-east-3"},

	{Project: "projectDA", Provider: defangv1.Provider_DIGITALOCEAN, Region: "us-central-1"},
	{Project: "projectDB", Provider: defangv1.Provider_DIGITALOCEAN, Region: "us-central-1"},

	{Project: "projectGA", Provider: defangv1.Provider_GCP, Region: "us-central-2"},
	{Project: "projectGB", Provider: defangv1.Provider_GCP, Region: "us-central-3"},

	{Project: "projectPlayground", Provider: defangv1.Provider_DEFANG, Region: "us-west-1"},
}

type mockActiveDeploymentsHandler struct {
	defangv1connect.UnimplementedFabricControllerHandler
	testDeploymentsData []*defangv1.Deployment
}

func (g *mockActiveDeploymentsHandler) ListDeployments(ctx context.Context, req *connect.Request[defangv1.ListDeploymentsRequest]) (*connect.Response[defangv1.ListDeploymentsResponse], error) {
	return connect.NewResponse(&defangv1.ListDeploymentsResponse{
		Deployments: g.testDeploymentsData,
	}), nil
}

func TestActiveDeployments(t *testing.T) {
	ctx := context.Background()

	fabricServer := &mockActiveDeploymentsHandler{}
	_, handler := defangv1connect.NewFabricControllerHandler(fabricServer)
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	url := strings.TrimPrefix(server.URL, "http://")
	grpcClient, _ := Connect(ctx, url)

	t.Run("no active deployments", func(t *testing.T) {
		fabricServer.testDeploymentsData = emptyDeployments
		stdout, _ := term.SetupTestTerm(t)

		err := DeploymentsList(ctx, defangv1.DeploymentType_DEPLOYMENT_TYPE_ACTIVE, "", *grpcClient, 10)
		if err != nil {
			t.Fatalf("DeploymentsList() error = %v", err)
		}

		receivedOutput := stdout.String()
		expectedOutput := "No deployments found"

		if !strings.Contains(receivedOutput, expectedOutput) {
			t.Errorf("Expected %s to contain %s", receivedOutput, expectedOutput)
		}
	})

	t.Run("some active deployments", func(t *testing.T) {
		fabricServer.testDeploymentsData = activeDeployments

		stdout, _ := term.SetupTestTerm(t)
		err := DeploymentsList(ctx, defangv1.DeploymentType_DEPLOYMENT_TYPE_ACTIVE, "", *grpcClient, 10)
		if err != nil {
			t.Fatalf("DeploymentsList() error = %v", err)
		}

		lines := strings.Split(stdout.String(), "\n")[2:] // Skip first two lines (space and header)

		// Verify each provider and project name exists in the output
		for _, deployment := range activeDeployments {
			match := false
			for _, line := range lines {
				if strings.Contains(line, strings.ToLower(deployment.Provider.String())) &&
					strings.Contains(line, deployment.Project) &&
					strings.Contains(line, deployment.Region) {
					match = true
					break
				}
			}
			if !match {
				t.Errorf("Missing expected output for provider %q and project %q", deployment.Provider.String(), deployment.Project)
			}
		}
	})
}
