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
	composeTypes "github.com/compose-spec/compose-go/v2/types"
)

type grpcDestroyMockHandler struct {
	defangv1connect.UnimplementedFabricControllerHandler
}

func (g *grpcDestroyMockHandler) Delete(context.Context, *connect.Request[defangv1.DeleteRequest]) (*connect.Response[defangv1.DeleteResponse], error) {
	return connect.NewResponse(&defangv1.DeleteResponse{
		Etag: "test-etag",
	}), nil
}

func (g *grpcDestroyMockHandler) GetServices(context.Context, *connect.Request[defangv1.GetServicesRequest]) (*connect.Response[defangv1.ListServicesResponse], error) {
	return connect.NewResponse(&defangv1.ListServicesResponse{
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
	loader := FakeLoader{ProjectName: "test-project"}
	grpcClient := Connect(url, loader)
	client := cliClient.PlaygroundProvider{GrpcClient: grpcClient}

	etag, err := client.Destroy(ctx, &defangv1.DestroyRequest{Project: "test-project"})
	if err != nil {
		t.Fatal(err)
	}

	if etag != "test-etag" {
		t.Fatalf("expected etag %q, got %q", "test-etag", etag)
	}
}

type FakeLoader struct {
	ProjectName string
}

func (f FakeLoader) LoadProject(ctx context.Context) (*composeTypes.Project, error) {
	return &composeTypes.Project{Name: f.ProjectName}, nil
}

func (f FakeLoader) LoadProjectName(ctx context.Context) (string, error) {
	return f.ProjectName, nil
}
