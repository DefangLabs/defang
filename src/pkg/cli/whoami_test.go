package cli

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
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
		Tenant:  "tenant-1",
		Account: "playground",
		Region:  "us-test-2",
		Tier:    defangv1.SubscriptionTier_PRO,
	}), nil
}

func TestWhoami(t *testing.T) {
	fabricServer := &grpcWhoamiMockHandler{}
	_, handler := defangv1connect.NewFabricControllerHandler(fabricServer)

	server := httptest.NewServer(handler)
	defer server.Close()

	ctx := context.Background()
	url := strings.TrimPrefix(server.URL, "http://")
	grpcClient := NewGrpcClient(ctx, url)
	client := cliClient.PlaygroundProvider{GrpcClient: grpcClient}

	got, err := Whoami(ctx, grpcClient, &client)
	if err != nil {
		t.Fatal(err)
	}

	// Playground provider is hardcoded to return "us-west-2" as the region
	want := "WhoAmI - \n\tProvider: playground\n\tTenant: tenant-1\n\tSubscription Tier: Pro\n\tRegion: us-west-2"

	if got != want {
		t.Errorf("Whoami() = %v, \nwant: %v", got, want)
	}
}
