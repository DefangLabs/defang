package cli

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/types"
)

type deployMock struct {
	client.MockProvider
}

func (d deployMock) Deploy(ctx context.Context, req *defangv1.DeployRequest) (*defangv1.DeployResponse, error) {
	if req.Compose == nil && req.Services == nil {
		return nil, errors.New("DeployRequest needs Compose or Services")
	}

	project, err := loader.LoadWithContext(ctx, types.ConfigDetails{ConfigFiles: []types.ConfigFile{{Content: req.Compose}}})
	if err != nil {
		return nil, err
	}

	var services []*defangv1.ServiceInfo
	for _, service := range project.Services {
		services = append(services, &defangv1.ServiceInfo{
			Service: &defangv1.Service{Name: service.Name},
		})
	}

	return &defangv1.DeployResponse{Services: services}, nil
}

func (b deployMock) AccountInfo(ctx context.Context) (client.AccountInfo, error) {
	return client.PlaygroundAccountInfo{}, nil
}

func (d deployMock) PrepareDomainDelegation(ctx context.Context, req client.PrepareDomainDelegationRequest) (*client.PrepareDomainDelegationResponse, error) {
	return &client.PrepareDomainDelegationResponse{
		NameServers:     []string{"ns1.example.com", "ns2.example.com"},
		DelegationSetId: "test-delegation-set-id",
	}, nil
}

func TestComposeUp(t *testing.T) {
	loader := compose.NewLoader(compose.WithPath("../../tests/testproj/compose.yaml"))
	proj, err := loader.LoadProject(context.Background())
	if err != nil {
		t.Fatalf("LoadProject() failed: %v", err)
	}

	gotContext := atomic.Bool{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("ComposeStart() failed: expected PUT request, got %s", r.Method)
		}
		gotContext.Store(true)
		w.WriteHeader(http.StatusOK) // return 200 OK same as S3
	}))
	t.Cleanup(server.Close)

	mc := client.MockFabricClient{DelegateDomain: "example.com"}
	mp := deployMock{MockProvider: client.MockProvider{UploadUrl: server.URL + "/"}}
	d, project, err := ComposeUp(context.Background(), proj, mc, mp, compose.UploadModeDigest, defangv1.DeploymentMode_DEVELOPMENT)
	if err != nil {
		t.Fatalf("ComposeUp() failed: %v", err)
	}
	if project == nil {
		t.Fatalf("ComposeUp() failed: project is nil")
	}
	if !gotContext.Load() {
		t.Errorf("ComposeStart() failed: did not get context")
	}
	if len(d.Services) != len(proj.Services) {
		t.Errorf("ComposeUp() failed: expected %d services, got %d", len(proj.Services), len(d.Services))
	}
	for _, service := range d.Services {
		if _, ok := proj.Services[service.Service.Name]; !ok {
			t.Errorf("ComposeUp() failed: service %s not found", service.Service.Name)
		}
	}
}
