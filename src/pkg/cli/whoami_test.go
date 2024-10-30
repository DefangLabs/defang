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
	}), nil
}

func TestWhoami(t *testing.T) {
	fabricServer := &grpcWhoamiMockHandler{}
	_, handler := defangv1connect.NewFabricControllerHandler(fabricServer)

	server := httptest.NewServer(handler)
	defer server.Close()

	ctx := context.Background()
	url := strings.TrimPrefix(server.URL, "http://")
	grpcClient := Connect(url, nil)
	client := cliClient.PlaygroundProvider{GrpcClient: grpcClient}

	got, err := Whoami(ctx, grpcClient, &client)
	if err != nil {
		t.Fatal(err)
	}

	// Playground provider is hardcoded to return "us-west-2" as the region
	want := `You are logged into playground region us-west-2 with tenant "tenant-1"`

	if got != want {
		t.Errorf("Whoami() = %v, want: %v", got, want)
	}
}
