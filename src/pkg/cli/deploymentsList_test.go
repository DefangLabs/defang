package cli

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
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
				Mode:              defangv1.DeploymentMode_MODE_UNSPECIFIED,
			},
		}
	}

	return connect.NewResponse(&defangv1.ListDeploymentsResponse{
		Deployments: deployments,
	}), nil
}

func TestDeploymentsList(t *testing.T) {
	ctx := t.Context()

	fabricServer := &mockListDeploymentsHandler{}
	_, handler := defangv1connect.NewFabricControllerHandler(fabricServer)
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	url := strings.TrimPrefix(server.URL, "http://")
	grpcClient := Connect(url, types.TenantUnset)

	t.Run("no deployments", func(t *testing.T) {
		stdout, _ := term.SetupTestTerm(t)
		err := DeploymentsList(ctx, grpcClient, ListDeploymentsParams{
			ListType:    defangv1.DeploymentType_DEPLOYMENT_TYPE_HISTORY,
			ProjectName: "empty",
			Limit:       10,
		})
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
		err := DeploymentsList(ctx, grpcClient, ListDeploymentsParams{
			ListType:    defangv1.DeploymentType_DEPLOYMENT_TYPE_HISTORY,
			ProjectName: "test",
			Limit:       10,
		})
		if err != nil {
			t.Fatalf("DeploymentsList() error = %v", err)
		}
		expectedOutput := "\x1b[1m\nPROJECTNAME  STACK  PROVIDER  ACCOUNTID   REGION     DEPLOYMENT  MODE              DEPLOYEDAT\x1b[0m" + `
test                defang    1234567890  us-test-2  a1b2c3      MODE_UNSPECIFIED  ` + time.Time{}.Local().Format(time.RFC3339) + "\n"

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
	ctx := t.Context()

	fabricServer := &mockActiveDeploymentsHandler{}
	_, handler := defangv1connect.NewFabricControllerHandler(fabricServer)
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	url := strings.TrimPrefix(server.URL, "http://")
	grpcClient := Connect(url, types.TenantUnset)

	t.Run("no active deployments", func(t *testing.T) {
		fabricServer.testDeploymentsData = nil
		stdout, _ := term.SetupTestTerm(t)

		err := DeploymentsList(ctx, grpcClient, ListDeploymentsParams{
			ListType:    defangv1.DeploymentType_DEPLOYMENT_TYPE_ACTIVE,
			ProjectName: "",
			Limit:       10,
		})
		if err != nil {
			t.Fatalf("DeploymentsList() error = %v", err)
		}

		receivedOutput := stdout.String()
		expectedOutput := "No deployments found"

		if !strings.Contains(receivedOutput, expectedOutput) {
			t.Errorf("Expected %s to contain %s", receivedOutput, expectedOutput)
		}
	})

	activeDeployments := []*defangv1.Deployment{
		{Project: "projectAA", Provider: defangv1.Provider_AWS, Region: "us-east-1"},
		{Project: "projectAB", Provider: defangv1.Provider_AWS, Region: "us-east-2"},
		{Project: "projectAC", Provider: defangv1.Provider_AWS, Region: "us-east-3"},

		{Project: "projectDA", Provider: defangv1.Provider_DIGITALOCEAN, Region: "us-central-1"},
		{Project: "projectDB", Provider: defangv1.Provider_DIGITALOCEAN, Region: "us-central-1"},

		{Project: "projectGA", Provider: defangv1.Provider_GCP, Region: "us-central-2"},
		{Project: "projectGB", Provider: defangv1.Provider_GCP, Region: "us-central-3"},

		{Project: "projectPlayground", Provider: defangv1.Provider_DEFANG, Region: "us-west-1"},
	}

	t.Run("some active deployments", func(t *testing.T) {
		fabricServer.testDeploymentsData = activeDeployments

		stdout, _ := term.SetupTestTerm(t)
		err := DeploymentsList(ctx, grpcClient, ListDeploymentsParams{
			ListType:    defangv1.DeploymentType_DEPLOYMENT_TYPE_ACTIVE,
			ProjectName: "",
			Limit:       10,
		})
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
