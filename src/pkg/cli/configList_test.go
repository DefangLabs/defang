package cli

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/DefangLabs/defang/src/protos/io/defang/v1/defangv1connect"
	"github.com/bufbuild/connect-go"
	"google.golang.org/protobuf/types/known/emptypb"
)

type grpcListSecretsMockHandler struct {
	defangv1connect.UnimplementedFabricControllerHandler
}

func (grpcListSecretsMockHandler) WhoAmI(context.Context, *connect.Request[emptypb.Empty]) (*connect.Response[defangv1.WhoAmIResponse], error) {
	return connect.NewResponse(&defangv1.WhoAmIResponse{}), nil
}

func (grpcListSecretsMockHandler) ListSecrets(ctx context.Context, req *connect.Request[defangv1.ListConfigsRequest]) (*connect.Response[defangv1.Secrets], error) {
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
	t.Cleanup(server.Close)

	url := strings.TrimPrefix(server.URL, "http://")
	grpcClient, err := Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	provider := cliClient.PlaygroundProvider{FabricClient: grpcClient}

	t.Run("no configs", func(t *testing.T) {
		stdout, _ := term.SetupTestTerm(t)

		err := ConfigList(ctx, "emptyconfigs", &provider)
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

		err := ConfigList(ctx, "test", &provider)
		if err != nil {
			t.Fatalf("ConfigList() error = %v", err)
		}

		expectedOutput := "\x1b[1m\nName  \x1b[0m" + `
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
