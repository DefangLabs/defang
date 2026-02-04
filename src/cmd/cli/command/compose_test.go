package command

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/bufbuild/connect-go"
)

func TestInitializeTailCmd(t *testing.T) {
	t.Run("", func(t *testing.T) {
		for _, cmd := range RootCmd.Commands() {
			if cmd.Use == "logs" {
				cmd.Execute()
				return
			}
		}
	})
}

func TestPrintPlaygroundPortalServiceURLs(t *testing.T) {
	defaultTerm := term.DefaultTerm
	t.Cleanup(func() {
		term.DefaultTerm = defaultTerm
	})

	var stdout, stderr bytes.Buffer
	term.DefaultTerm = term.NewTerm(os.Stdin, &stdout, &stderr)

	global.Stack.Provider = client.ProviderDefang
	global.Cluster = client.DefaultCluster
	printPlaygroundPortalServiceURLs([]*defangv1.ServiceInfo{
		{
			Service: &defangv1.Service{Name: "service1"},
		}})
	const want = ` * Monitor your services' status in the defang portal
   - https://portal.defang.io/service/service1
`
	if got := stdout.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

type unauthedMockFabricClient struct {
	client.MockFabricClient
}

func (c unauthedMockFabricClient) GetDefaultStack(context.Context, *defangv1.GetDefaultStackRequest) (*defangv1.GetStackResponse, error) {
	return nil, connect.NewError(connect.CodeUnauthenticated, nil)
}

func TestComposeConfig(t *testing.T) {
	// Test fix for https://github.com/DefangLabs/defang/issues/1894
	global.Client = unauthedMockFabricClient{}
	t.Cleanup(func() {
		global.Client = nil
	})

	t.Run("Unauth OK", func(t *testing.T) {
		t.Chdir("testdata/without-stack")
		cmd := makeComposeConfigCmd()
		err := cmd.Execute()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("Unauth OK - with stack", func(t *testing.T) {
		t.Chdir("testdata/with-project-and-stack")
		cmd := makeComposeConfigCmd()
		err := cmd.Execute()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}
