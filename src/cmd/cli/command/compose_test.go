package command

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func TestComposeDownRemoveDetachConflict(t *testing.T) {
	cmd := makeComposeDownCmd()
	cmd.SetArgs([]string{"--remove", "--detach"})
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when combining --remove and --detach, got nil")
	}
	if !strings.Contains(err.Error(), "cannot use --remove with --detach") {
		t.Errorf("unexpected error: %v", err)
	}
}

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
	global.FabricAddr = client.DefaultFabricAddr
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

func TestComposeLint(t *testing.T) {
	defaultTerm := term.DefaultTerm
	t.Cleanup(func() {
		term.DefaultTerm = defaultTerm
	})

	t.Run("Valid", func(t *testing.T) {
		t.Chdir("testdata/without-stack")
		var stdout, stderr bytes.Buffer
		term.DefaultTerm = term.NewTerm(os.Stdin, &stdout, &stderr)

		cmd := makeComposeLintCmd()
		err := cmd.Execute()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if !strings.Contains(stdout.String(), "Compose file is valid") {
			t.Fatalf("expected valid lint output, got stdout %q stderr %q", stdout.String(), stderr.String())
		}
	})

	t.Run("Invalid", func(t *testing.T) {
		t.Chdir("testdata/lint-invalid")
		var stdout, stderr bytes.Buffer
		term.DefaultTerm = term.NewTerm(os.Stdin, &stdout, &stderr)

		cmd := makeComposeLintCmd()
		err := cmd.Execute()
		if err == nil {
			t.Fatal("expected lint error")
		}
		if !strings.Contains(err.Error(), "compose file has errors:") {
			t.Fatalf("expected error heading, got error %q stdout %q stderr %q", err.Error(), stdout.String(), stderr.String())
		}
		if !strings.Contains(err.Error(), "unsupported compose directive: hostname; use 'domainname' instead") {
			t.Fatalf("expected remediation hint, got error %q", err.Error())
		}
	})
}
