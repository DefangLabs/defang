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

type mockActiveDeploymentsHandler struct {
	defangv1connect.UnimplementedFabricControllerHandler
}

var emptyDeployments map[string]*defangv1.ProjectNames
var activeDeployments = map[string]*defangv1.ProjectNames{
	defangv1.Provider_AWS.String():          {Values: []string{"projectAA", "projectAB", "projectAC"}},
	defangv1.Provider_DEFANG.String():       {Values: []string{"projectPlayground"}},
	defangv1.Provider_DIGITALOCEAN.String(): {Values: []string{"projectDA", "projectDB"}},
	defangv1.Provider_GCP.String():          {Values: []string{"projectGA", "projectGB"}},
}
var testDeploymentsData = emptyDeployments

func (g *mockActiveDeploymentsHandler) GetActiveDeployments(ctx context.Context, req *connect.Request[defangv1.ActiveDeploymentsRequest]) (*connect.Response[defangv1.ActiveDeploymentsResponse], error) {
	return connect.NewResponse(&defangv1.ActiveDeploymentsResponse{
		Deployments: testDeploymentsData,
	}), nil
}

func TestActiveDeployments(t *testing.T) {
	ctx := context.Background()

	fabricServer := &mockActiveDeploymentsHandler{}
	_, handler := defangv1connect.NewFabricControllerHandler(fabricServer)
	server := httptest.NewServer(handler)
	t.Cleanup(func() {
		server.Close()
	})

	url := strings.TrimPrefix(server.URL, "http://")
	client := NewGrpcClient(ctx, url)

	t.Run("no active deployments", func(t *testing.T) {
		testDeploymentsData = emptyDeployments
		stdout, _ := term.SetupTestTerm(t)

		err := ActiveDeployments(ctx, client)
		if err != nil {
			t.Fatalf("ActiveDeployments() error = %v", err)
		}

		receivedOutput := stdout.String()
		expectedOutput := "No active deployments"

		if !strings.Contains(receivedOutput, expectedOutput) {
			t.Errorf("Expected %s to contain %s", receivedOutput, expectedOutput)
		}
	})

	t.Run("some active deployments", func(t *testing.T) {
		testDeploymentsData = activeDeployments

		stdout, _ := term.SetupTestTerm(t)
		err := ActiveDeployments(ctx, client)
		if err != nil {
			t.Fatalf("ActiveDeployments() error = %v", err)
		}

		lines := strings.Split(stdout.String(), "\n")[2:] // Skip first two lines (space and header)

		// Verify each provider and project name exists in the output
		for provider, projectNames := range activeDeployments {
			for _, projectName := range projectNames.Values {
				match := false
				for _, line := range lines {
					if strings.Contains(line, provider) && strings.Contains(line, projectName) {
						match = true
						break
					}
				}
				if !match {
					t.Errorf("Missing expected output for provider %q and project %q", provider, projectName)
				}
			}
		}
	})
}
