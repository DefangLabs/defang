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
	"AWS":          {Values: []string{"projectAA", "projectAB", "projectAC"}},
	"DEFANG":       {Values: []string{"projectPlayground"}},
	"DIGITALOCEAN": {Values: []string{"projectDA", "projectDB"}},
	"GCP":          {Values: []string{"projectGA", "projectGB"}},
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

		if !strings.Contains(stdout.String(), expectedOutput) {
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
		receivedLines := strings.Split(stdout.String(), "\n")
		receivedLines = receivedLines[2:] // remove the space and header

		// check every entry in activeDeployments is found in the output
		for provider, projectNames := range activeDeployments {
			for _, projectName := range projectNames.Values {
				found := false
				for i, line := range receivedLines {
					if strings.Contains(line, provider) && strings.Contains(line, projectName) {
						found = true
						// remove the line from receivedLines
						receivedLines = append(receivedLines[:i], receivedLines[i+1:]...)
						break
					}
				}

				if !found {
					t.Errorf("Expected %s - %s to be found in output", provider, projectName)
				}
			}
		}
	})
}
