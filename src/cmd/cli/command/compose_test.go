package command

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"

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

func TestResolveTTL(t *testing.T) {
	now := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		stackTTL string // simulates a DEFANG_TTL stack-file variable loaded by LoadSession
		flagTTL  string
		flagSet  bool
		want     string
		wantErr  bool
	}{
		{name: "no TTL anywhere", want: ""},
		{name: "stack file only", stackTTL: "7d", want: "7d"},
		{name: "flag only", flagTTL: "12h", flagSet: true, want: "12h"},
		{name: "flag wins over stack file", stackTTL: "7d", flagTTL: "12h", flagSet: true, want: "12h"},
		{name: "flag never cancels stack file TTL", stackTTL: "7d", flagTTL: "never", flagSet: true, want: "never"},
		{name: "flag timestamp becomes a duration", flagTTL: "2026-08-17T12:00:00Z", flagSet: true, want: "24h0m0s"},
		{name: "invalid flag value", flagTTL: "1w", flagSet: true, wantErr: true},
		{name: "invalid stack file value", stackTTL: "soon", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("DEFANG_TTL", tt.stackTTL)
			got, err := resolveTTL(tt.flagTTL, tt.flagSet, now)
			if (err != nil) != tt.wantErr {
				t.Fatalf("resolveTTL() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && got != tt.want {
				t.Errorf("resolveTTL() = %q, want %q", got, tt.want)
			}
		})
	}
}
