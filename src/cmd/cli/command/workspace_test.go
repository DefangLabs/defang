package command

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/auth"
	"github.com/DefangLabs/defang/src/pkg/cli"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1/defangv1connect"
)

func setupWorkspaceTestServers(t *testing.T) (clusterURL string) {
	t.Helper()

	mockService := &mockFabricService{}
	_, handler := defangv1.NewFabricControllerHandler(mockService)

	fabricServer := httptest.NewServer(handler)
	t.Cleanup(fabricServer.Close)

	userinfoServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/userinfo" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"allTenants":[
				{"id":"ws-1","name":"Workspace One"},
				{"id":"ws-2","name":"Workspace Two"}
			],
			"userinfo":{"email":"cli@example.com","name":"CLI Tester"}
		}`))
	}))
	t.Cleanup(userinfoServer.Close)

	openAuthClient := auth.OpenAuthClient
	t.Cleanup(func() {
		auth.OpenAuthClient = openAuthClient
	})
	auth.OpenAuthClient = auth.NewClient("testclient", userinfoServer.URL)
	t.Setenv("DEFANG_ACCESS_TOKEN", "token-123")

	return fabricServer.URL
}

func TestWorkspaceListJSON(t *testing.T) {
	stdout, _ := term.SetupTestTerm(t)
	term.DefaultTerm.ForceColor(false)

	clusterURL := setupWorkspaceTestServers(t)

	oldGlobal := global
	t.Cleanup(func() { global = oldGlobal })

	// Reset stack name to prevent loading stack files
	global.Stack.Name = ""
	global.Tenant = "ws-2"

	if err := testCommand([]string{"workspace", "ls", "--json", "--non-interactive"}, clusterURL); err != nil {
		t.Fatalf("workspace ls --json failed: %v", err)
	}

	var rows []cli.WorkspaceRow
	if err := json.Unmarshal(stdout.Bytes(), &rows); err != nil {
		t.Fatalf("failed to unmarshal JSON output: %v\noutput: %s", err, stdout.String())
	}

	if len(rows) != 2 {
		t.Fatalf("expected 2 workspaces, got %d", len(rows))
	}

	if rows[0].ID != "" || rows[1].ID != "" {
		t.Fatalf("expected IDs to be omitted in non-verbose JSON output, got %+v", rows)
	}

	if !rows[1].Current || rows[0].Current {
		t.Fatalf("expected ws-2 to be current; rows=%+v", rows)
	}
}

func TestWorkspaceListVerboseTable(t *testing.T) {
	stdout, _ := term.SetupTestTerm(t)
	term.DefaultTerm.ForceColor(false)

	clusterURL := setupWorkspaceTestServers(t)

	oldGlobal := global
	t.Cleanup(func() { global = oldGlobal })

	global.Tenant = "Workspace One"
	// Reset stack name to prevent loading stack files
	global.Stack.Name = ""

	if err := testCommand([]string{"workspace", "ls", "--verbose", "--json=false", "--non-interactive"}, clusterURL); err != nil {
		t.Fatalf("workspace ls --verbose failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "NAME") || !strings.Contains(output, "CURRENT") {
		t.Fatalf("table header missing in output: %q", output)
	}
	if !strings.Contains(output, "Workspace One") || !strings.Contains(output, "ws-1") || !strings.Contains(output, "true") {
		t.Fatalf("expected Workspace One row with ID and current flag, got: %q", output)
	}
	if !strings.Contains(output, "Workspace Two") || !strings.Contains(output, "ws-2") {
		t.Fatalf("expected Workspace Two row with ID, got: %q", output)
	}
}
