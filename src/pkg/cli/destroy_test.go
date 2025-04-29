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

type grpcDestroyMockHandler struct {
	defangv1connect.UnimplementedFabricControllerHandler
}

func (grpcDestroyMockHandler) WhoAmI(context.Context, *connect.Request[emptypb.Empty]) (*connect.Response[defangv1.WhoAmIResponse], error) {
	return connect.NewResponse(&defangv1.WhoAmIResponse{}), nil
}

func (grpcDestroyMockHandler) Delete(context.Context, *connect.Request[defangv1.DeleteRequest]) (*connect.Response[defangv1.DeleteResponse], error) {
	return connect.NewResponse(&defangv1.DeleteResponse{
		Etag: "test-etag",
	}), nil
}

func (grpcDestroyMockHandler) GetServices(context.Context, *connect.Request[defangv1.GetServicesRequest]) (*connect.Response[defangv1.GetServicesResponse], error) {
	return connect.NewResponse(&defangv1.GetServicesResponse{
		Project: "tenantx",
		Services: []*defangv1.ServiceInfo{
			{
				Service: &defangv1.Service{Name: "test-service"},
			},
		},
	}), nil
}

func TestDestroy(t *testing.T) {
	fabricServer := &grpcDestroyMockHandler{}
	_, handler := defangv1connect.NewFabricControllerHandler(fabricServer)

	server := httptest.NewServer(handler)
	defer server.Close()

	ctx := context.Background()
	url := strings.TrimPrefix(server.URL, "http://")
	grpcClient, _ := Connect(ctx, url)
	client := cliClient.PlaygroundProvider{FabricClient: grpcClient}

	etag, err := client.Destroy(ctx, &defangv1.DestroyRequest{Project: "test-project"})
	if err != nil {
		t.Fatal(err)
	}

	if etag != "test-etag" {
		t.Fatalf("expected etag %q, got %q", "test-etag", etag)
	}
}
