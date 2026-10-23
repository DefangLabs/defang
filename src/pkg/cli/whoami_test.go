package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/DefangLabs/defang/src/pkg/auth"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/tokenstore"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/DefangLabs/defang/src/protos/io/defang/v1/defangv1connect"
	"google.golang.org/protobuf/types/known/emptypb"
)

type grpcWhoamiMockHandler struct {
	defangv1connect.UnimplementedFabricControllerHandler
}

func (g *grpcWhoamiMockHandler) WhoAmI(context.Context, *connect.Request[emptypb.Empty]) (*connect.Response[defangv1.WhoAmIResponse], error) {
	return connect.NewResponse(&defangv1.WhoAmIResponse{
		Tenant:            "tenant-1",
		ProviderAccountId: "playground",
		Region:            "us-test-2",
		Tier:              defangv1.SubscriptionTier_PRO,
	}), nil
}

func TestWhoami(t *testing.T) {
	mockService := &grpcWhoamiMockHandler{}
	_, handler := defangv1connect.NewFabricControllerHandler(mockService)

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	ctx := t.Context()
	url := strings.TrimPrefix(server.URL, "http://")
	const requestedTenant = "tenant-1"
	grpcClient, _ := ConnectWithTenant(ctx, url, requestedTenant)
	provider := client.PlaygroundProvider{FabricClient: grpcClient}

	userInfo := &auth.UserInfo{
		AllTenants: []auth.WorkspaceInfo{
			{ID: "tenant-1", Name: "Tenant One"},
		},
		User: auth.UserDetails{
			Email: "user@example.com",
			Name:  "Test User",
		},
	}

	got, err := Whoami(ctx, grpcClient, &provider, userInfo, types.TenantNameOrID(requestedTenant))
	if err != nil {
		t.Fatal(err)
	}

	want := ShowAccountData{
		Provider:       client.ProviderDefang,
		SubscriberTier: "Pro",
		Region:         "us-west-2",
		Workspace:      "Tenant One",
		Tenant:         "tenant-1",
		TenantID:       "tenant-1",
		Email:          "user@example.com",
		Name:           "Test User",
	}

	if got != want {
		t.Errorf("Whoami() = %v, \nwant: %v", got, want)
	}
}

func TestFetchAccountInfo(t *testing.T) {
	mockService := &grpcWhoamiMockHandler{}
	_, handler := defangv1connect.NewFabricControllerHandler(mockService)

	fabricServer := httptest.NewServer(handler)
	t.Cleanup(fabricServer.Close)

	ctx := t.Context()
	fabricAddr := strings.TrimPrefix(fabricServer.URL, "http://")
	grpcClient, _ := ConnectWithTenant(ctx, fabricAddr, "tenant-1")

	t.Run("non-interactive skips the userinfo fetch", func(t *testing.T) {
		got, err := FetchAccountInfo(ctx, grpcClient, nil, fabricAddr, "tenant-1", false)
		if err != nil {
			t.Fatal(err)
		}
		if got.Workspace != "tenant-1" || got.Email != "" {
			t.Errorf("expected the raw tenant id with no userinfo, got: %+v", got)
		}
	})

	t.Run("interactive fetches userinfo and resolves the workspace name", func(t *testing.T) {
		t.Setenv("DEFANG_ACCESS_TOKEN", "") // GetExistingToken prefers this env var over TokenStore

		userinfoServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"allTenants":[{"id":"tenant-1","name":"Tenant One"}],
				"userinfo":{"email":"user@example.com","name":"Test User"}
			}`))
		}))
		t.Cleanup(userinfoServer.Close)

		originalAuthClient := auth.OpenAuthClient
		auth.OpenAuthClient = auth.NewClient("testclient", userinfoServer.URL)
		t.Cleanup(func() { auth.OpenAuthClient = originalAuthClient })

		originalTokenStore := client.TokenStore
		client.TokenStore = &tokenstore.LocalDirTokenStore{Dir: t.TempDir()}
		t.Cleanup(func() { client.TokenStore = originalTokenStore })
		if err := client.TokenStore.Save(client.TokenStorageName(fabricAddr), "test-token"); err != nil {
			t.Fatalf("failed to seed token store: %v", err)
		}

		got, err := FetchAccountInfo(ctx, grpcClient, nil, fabricAddr, "tenant-1", true)
		if err != nil {
			t.Fatal(err)
		}
		if got.Workspace != "Tenant One" || got.Email != "user@example.com" {
			t.Errorf("expected the resolved workspace name and userinfo, got: %+v", got)
		}
	})
}
