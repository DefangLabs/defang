package cli

import (
	"context"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type mockComposeDown struct {
	client.MockProvider
	MockAccountInfo func(ctx context.Context) (*client.AccountInfo, error)
	MockDestroy     func(ctx context.Context, req *defangv1.DestroyRequest) (types.ETag, error)
	MockDelete      func(ctx context.Context, req *defangv1.DeleteRequest) (*defangv1.DeleteResponse, error)
	request         map[string]interface{}
}

func (m mockComposeDown) AccountInfo(
	ctx context.Context,
) (*client.AccountInfo, error) {
	return m.MockAccountInfo(ctx)
}

func (m mockComposeDown) Destroy(
	ctx context.Context,
	req *defangv1.DestroyRequest,
) (types.ETag, error) {
	return m.MockDestroy(ctx, req)
}

func (m mockComposeDown) Delete(
	ctx context.Context,
	req *defangv1.DeleteRequest,
) (*defangv1.DeleteResponse, error) {
	return m.MockDelete(ctx, req)
}

func TestComposeDown(t *testing.T) {
	loader := compose.NewLoader(compose.WithPath("../../testdata/testproj/compose.yaml"))
	proj, _, err := loader.LoadProject(context.Background())
	if err != nil {
		t.Fatalf("LoadProject() failed: %v", err)
	}

	mockClient := client.MockFabricClient{DelegateDomain: "example.com"}
	var mockProvider mockComposeDown
	mockProvider = mockComposeDown{
		MockProvider: client.MockProvider{},
		MockAccountInfo: func(ctx context.Context) (*client.AccountInfo, error) {
			return &client.AccountInfo{}, nil
		},
		MockDestroy: func(ctx context.Context, req *defangv1.DestroyRequest) (types.ETag, error) {
			mockProvider.request["DestroyRequest"] = req
			return "eTagDestroy", nil
		},
		MockDelete: func(ctx context.Context, req *defangv1.DeleteRequest) (*defangv1.DeleteResponse, error) {
			mockProvider.request["DeleteRequest"] = req
			return &defangv1.DeleteResponse{Etag: "eTagDelete"}, nil
		},
		request: make(map[string]interface{}),
	}

	t.Run("Expect `Provider.Destroy` to be called when no specific services are specified",
		func(t *testing.T) {
			etag, err := ComposeDown(context.Background(), proj.Name, mockClient, mockProvider)
			if err != nil {
				t.Fatalf("ComposeDown() failed: %v", err)
			}
			if etag != "eTagDestroy" {
				t.Errorf("ComposeDown() failed: expected eTagSomething, got %s", etag)
			}
			if req, ok := mockProvider.request["DestroyRequest"]; ok {
				req, err := req.(*defangv1.DestroyRequest)
				if !err {
					t.Errorf("ComposeDown() failed: expected DestroyRequest, got %v", req)
				}
				if req.Project != proj.Name {
					t.Errorf("ComposeDown() failed: expected project %s, got %s", proj.Name, req.Project)
				}
			}
		})

	t.Run("Expect `Provider.Delete` to be called when project and services are specified",
		func(t *testing.T) {
			services := make([]string, 0, len(proj.Services))
			for _, service := range proj.Services {
				services = append(services, service.Name)
			}
			etag, err := ComposeDown(context.Background(), proj.Name, mockClient, mockProvider, services...)

			if err != nil {
				t.Fatalf("ComposeDown() failed: %v", err)
			}
			if etag != "eTagDelete" {
				t.Errorf("ComposeDown() failed: expected eTagSomething, got %s", etag)
			}
			if req, ok := mockProvider.request["DeleteRequest"]; ok {
				req, err := req.(*defangv1.DeleteRequest)
				if !err {
					t.Errorf("ComposeDown() failed: expected DestroyRequest, got %v", req)
				}
				if req.Project != proj.Name || len(req.Names) != len(services) {
					t.Errorf("ComposeDown() failed: expected project %s, got %s", proj.Name, req.Project)
				}
			}
		})
}
