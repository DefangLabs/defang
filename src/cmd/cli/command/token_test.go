package command

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DefangLabs/defang/src/pkg/auth"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/tokenstore"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1/defangv1connect"
	"github.com/golang-jwt/jwt/v5"
)

func TestTokenNonInteractiveReusesLoginAndSaves(t *testing.T) {
	stdout, _ := term.SetupTestTerm(t)
	term.DefaultTerm.ForceColor(false)

	mockService := &mockFabricService{tenantId: "workspace-id"}
	_, handler := defangv1.NewFabricControllerHandler(mockService)
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	originalAuthClient := auth.OpenAuthClient
	auth.OpenAuthClient = auth.NewClient("testclient", "https://auth.example.com")
	t.Cleanup(func() { auth.OpenAuthClient = originalAuthClient })

	loginToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Issuer: "https://auth.example.com",
	}).SignedString([]byte("test-key"))
	if err != nil {
		t.Fatal(err)
	}

	originalTokenStore := client.TokenStore
	client.TokenStore = &tokenstore.LocalDirTokenStore{Dir: t.TempDir()}
	t.Cleanup(func() { client.TokenStore = originalTokenStore })
	fabricAddr := strings.TrimPrefix(server.URL, "http://")
	if err := client.SaveAccessToken(fabricAddr, loginToken); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DEFANG_ACCESS_TOKEN", "")

	oldGlobal := global
	t.Cleanup(func() {
		global = oldGlobal
		_ = tokenCmd.Flags().Set("save", "false")
	})
	global.Stack.Name = ""

	err = testCommand(t, []string{
		"token",
		"--workspace", "workspace-id",
		"--scope", "admin",
		"--expires", "8760h",
		"--save",
		"--non-interactive",
	}, server.URL)
	if err != nil {
		t.Fatalf("token command failed: %v", err)
	}

	if mockService.tokenRequest == nil {
		t.Fatal("token command did not call Fabric Token")
	}
	if mockService.tokenRequest.Assertion != loginToken {
		t.Error("token command did not reuse the stored OpenAuth login")
	}
	if mockService.tokenRequest.Tenant != "workspace-id" {
		t.Errorf("tenant = %q, want %q", mockService.tokenRequest.Tenant, "workspace-id")
	}
	if mockService.tokenRequest.ExpiresIn != uint32((365 * 24 * time.Hour).Seconds()) {
		t.Errorf("expires_in = %d, want one year", mockService.tokenRequest.ExpiresIn)
	}

	saved, err := client.TokenStore.Load(client.TokenStorageName(fabricAddr))
	if err != nil {
		t.Fatal(err)
	}
	if saved != "defang_generated" {
		t.Errorf("saved token = %q, want generated token", saved)
	}
	if strings.Contains(stdout.String(), "defang_generated") {
		t.Error("saved token was printed to stdout")
	}
}
