package cli

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/DefangLabs/defang/src/protos/io/defang/v1/defangv1connect"
	"github.com/bufbuild/connect-go"
)

type grpcListSecretsMockHandler struct {
	defangv1connect.UnimplementedFabricControllerHandler
}

func (g *grpcListSecretsMockHandler) ListSecrets(ctx context.Context, req *connect.Request[defangv1.ListConfigsRequest]) (*connect.Response[defangv1.Secrets], error) {
	var names []string

	if req.Msg.Project == "emptyconfigs" {
		names = []string{}
	} else {
		names = []string{
			"foo",
			"bar",
		}
	}
	return connect.NewResponse(&defangv1.Secrets{
		Names: names,
	}), nil
}

func TestConfigList(t *testing.T) {
	ctx := context.Background()

	fabricServer := &grpcListSecretsMockHandler{}
	_, handler := defangv1connect.NewFabricControllerHandler(fabricServer)
	server := httptest.NewServer(handler)
	t.Cleanup(func() {
		server.Close()
	})

	url := strings.TrimPrefix(server.URL, "http://")
	grpcClient := NewGrpcClient(ctx, url)
	provider := cliClient.PlaygroundProvider{GrpcClient: grpcClient}

	t.Run("no configs", func(t *testing.T) {
		stdout, _ := term.SetupTestTerm(t)

		loader := client.MockLoader{Project: &compose.Project{Name: "emptyconfigs"}}
		err := ConfigList(ctx, loader, &provider)
		if err != nil {
			t.Fatalf("ConfigList() error = %v", err)
		}

		receivedOutput := stdout.String()
		expectedOutput := "No configs"

		if !strings.Contains(stdout.String(), expectedOutput) {
			t.Errorf("Expected %s to contain %s", receivedOutput, expectedOutput)
		}
	})

	t.Run("some configs", func(t *testing.T) {
		stdout, _ := term.SetupTestTerm(t)

		loader := client.MockLoader{Project: &compose.Project{Name: "test"}}
		err := ConfigList(ctx, loader, &provider)
		if err != nil {
			t.Fatalf("ConfigList() error = %v", err)
		}

		expectedOutput := `Name
foo
bar
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
