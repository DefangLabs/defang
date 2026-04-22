package command

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"testing"

	"connectrpc.com/connect"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/logs"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
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
	oldStdout := os.Stdout
	t.Cleanup(func() {
		term.DefaultTerm = defaultTerm
		os.Stdout = oldStdout
	})

	// Capture slog output via term logger
	var termBuf, stderr bytes.Buffer
	term.DefaultTerm = term.NewTerm(os.Stdin, &termBuf, &stderr)
	slog.SetDefault(logs.NewTermLogger(term.DefaultTerm))

	// Capture fmt.Println output via os.Pipe
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	global.Stack.Provider = client.ProviderDefang
	global.FabricAddr = client.DefaultFabricAddr
	printPlaygroundPortalServiceURLs([]*defangv1.ServiceInfo{
		{
			Service: &defangv1.Service{Name: "service1"},
		}})

	w.Close()
	var stdoutBuf bytes.Buffer
	stdoutBuf.ReadFrom(r)

	const wantSlog = " * Monitor your services' status in the defang portal\n"
	if got := termBuf.String(); got != wantSlog {
		t.Errorf("slog output: got %q, want %q", got, wantSlog)
	}
	const wantStdout = "   - https://portal.defang.io/service/service1\n"
	if got := stdoutBuf.String(); got != wantStdout {
		t.Errorf("stdout output: got %q, want %q", got, wantStdout)
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
