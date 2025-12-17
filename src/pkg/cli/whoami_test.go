package cli

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/auth"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/DefangLabs/defang/src/protos/io/defang/v1/defangv1connect"
	"github.com/bufbuild/connect-go"
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
	client := cliClient.PlaygroundProvider{FabricClient: grpcClient}

	userInfo := &auth.UserInfo{
		AllTenants: []auth.WorkspaceInfo{
			{ID: "tenant-1", Name: "Tenant One"},
		},
		User: auth.UserDetails{
			Email: "user@example.com",
			Name:  "Test User",
		},
	}

	got, err := Whoami(ctx, grpcClient, &client, userInfo, types.TenantNameOrID(requestedTenant), true)
	if err != nil {
		t.Fatal(err)
	}

	want := ShowAccountData{
		Provider:       cliClient.ProviderDefang,
		SubscriberTier: defangv1.SubscriptionTier_PRO,
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

func TestParseTokenClaims(t *testing.T) {
	token := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJmYWJyaWMtZGV2IiwiYXVkIjoiZGVmYW5nLWF1ZCIsInN1YiI6InVzZXIxMjMifQ.c2lnbmF0dXJl" // #nosec G101 -- test JWT token, not a real credential
	claims, err := ParseTokenClaims(token)
	if err != nil {
		t.Fatalf("ParseTokenClaims() unexpected error: %v", err)
	}

	if claims.Subject != "user123" {
		t.Fatalf("ParseTokenClaims().Subject = %q, want %q", claims.Subject, "user123")
	}
	if claims.Issuer != "fabric-dev" {
		t.Fatalf("ParseTokenClaims().Issuer = %q, want %q", claims.Issuer, "fabric-dev")
	}
}
