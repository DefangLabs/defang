package command

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/auth"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/tokenstore"
	"github.com/DefangLabs/defang/src/pkg/track"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1/defangv1connect"
	"github.com/spf13/cobra"
)

// setupLoginTestServers is like setupWorkspaceTestServers, but stores the access token via
// client.TokenStore instead of DEFANG_ACCESS_TOKEN, so a tenant selection that differs from the
// server's default tenant is not rejected as a fixed-workspace mismatch (see
// client.UsingAccessTokenEnv).
func setupLoginTestServers(t *testing.T) (clusterURL string) {
	t.Helper()
	t.Setenv("DEFANG_ACCESS_TOKEN", "") // GetExistingToken prefers this env var over TokenStore

	mockService := &mockFabricService{tenantId: "ws-2"}
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
	t.Cleanup(func() { auth.OpenAuthClient = openAuthClient })
	auth.OpenAuthClient = auth.NewClient("testclient", userinfoServer.URL)

	originalTokenStore := client.TokenStore
	client.TokenStore = &tokenstore.LocalDirTokenStore{Dir: t.TempDir()}
	t.Cleanup(func() { client.TokenStore = originalTokenStore })

	fabricAddr := strings.TrimPrefix(fabricServer.URL, "http://")
	if err := client.TokenStore.Save(client.TokenStorageName(fabricAddr), "test-token"); err != nil {
		t.Fatalf("failed to seed token store: %v", err)
	}

	return fabricServer.URL
}

func TestPrintActiveWorkspace(t *testing.T) {
	stdout, _ := term.SetupTestTerm(t)
	term.DefaultTerm.ForceColor(false)

	clusterURL := setupLoginTestServers(t)

	oldGlobal := global
	oldTracker := track.Tracker
	t.Cleanup(func() {
		global = oldGlobal
		track.Tracker = oldTracker
	})
	global.Stack.Name = "" // prevent loading stack files
	global.FabricAddr = strings.TrimPrefix(clusterURL, "http://")
	global.HasTty = true

	t.Run("no explicit selection resolves to the server's default tenant", func(t *testing.T) {
		stdout.Reset()
		global.TenantSelection = ""

		cmd := &cobra.Command{}
		cmd.SetContext(t.Context())
		printActiveWorkspace(cmd)

		if global.Client == nil {
			t.Fatal("expected global.Client to be reconnected with the new token")
		}
		if output := stdout.String(); !strings.Contains(output, "Workspace Two") {
			t.Fatalf("expected active workspace name in output, got: %q", output)
		}
	})

	t.Run("explicit selection is resolved by name", func(t *testing.T) {
		stdout.Reset()
		global.TenantSelection = "Workspace One"

		cmd := &cobra.Command{}
		cmd.SetContext(t.Context())
		printActiveWorkspace(cmd)

		if output := stdout.String(); !strings.Contains(output, "Workspace One") {
			t.Fatalf("expected selected workspace name in output, got: %q", output)
		}
	})

	t.Run("userinfo fetch failure falls back to the raw tenant id", func(t *testing.T) {
		stdout.Reset()
		global.TenantSelection = ""

		failingUserinfo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// A 4xx status (unlike 5xx) isn't retried by the retryablehttp client underneath
			// auth.FetchUserInfo, keeping this test fast.
			http.Error(w, "userinfo unavailable", http.StatusBadRequest)
		}))
		t.Cleanup(failingUserinfo.Close)

		originalAuthClient := auth.OpenAuthClient
		auth.OpenAuthClient = auth.NewClient("testclient", failingUserinfo.URL)
		t.Cleanup(func() { auth.OpenAuthClient = originalAuthClient })

		cmd := &cobra.Command{}
		cmd.SetContext(t.Context())
		printActiveWorkspace(cmd)

		if output := stdout.String(); !strings.Contains(output, "ws-2") {
			t.Fatalf("expected fallback to the raw tenant id in output, got: %q", output)
		}
	})
}
