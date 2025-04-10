package cli

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type mockDeployProvider struct {
	client.MockProvider
	deploymentStatus error
	subscribeStream  *client.MockWaitStream[defangv1.SubscribeResponse]
	tailStream       *client.MockWaitStream[defangv1.TailResponse]
}

func (d mockDeployProvider) Deploy(ctx context.Context, req *defangv1.DeployRequest) (*defangv1.DeployResponse, error) {
	return d.Preview(ctx, req)
}

func (mockDeployProvider) Preview(ctx context.Context, req *defangv1.DeployRequest) (*defangv1.DeployResponse, error) {
	if req.Compose == nil && req.Services == nil {
		return nil, errors.New("DeployRequest needs Compose or Services")
	}

	project, err := compose.LoadFromContent(ctx, req.Compose, "")
	if err != nil {
		return nil, err
	}

	etag := pkg.RandomID()
	var services []*defangv1.ServiceInfo
	for _, service := range project.Services {
		services = append(services, &defangv1.ServiceInfo{
			Service: &defangv1.Service{Name: service.Name},
			Etag:    etag,
		})
	}

	return &defangv1.DeployResponse{Services: services, Etag: etag}, ctx.Err()
}

func (m *mockDeployProvider) Subscribe(ctx context.Context, req *defangv1.SubscribeRequest) (client.ServerStream[defangv1.SubscribeResponse], error) {
	m.subscribeStream = client.NewMockWaitStream[defangv1.SubscribeResponse]()
	return m.subscribeStream, ctx.Err()
}

func (m *mockDeployProvider) QueryLogs(ctx context.Context, req *defangv1.TailRequest) (client.ServerStream[defangv1.TailResponse], error) {
	m.tailStream = client.NewMockWaitStream[defangv1.TailResponse]()
	return m.tailStream, ctx.Err()
}

func (m mockDeployProvider) GetDeploymentStatus(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return context.Cause(ctx)
	default:
		return m.deploymentStatus
	}
}

func (mockDeployProvider) AccountInfo(ctx context.Context) (client.AccountInfo, error) {
	return client.PlaygroundAccountInfo{}, ctx.Err()
}

func (mockDeployProvider) PrepareDomainDelegation(ctx context.Context, req client.PrepareDomainDelegationRequest) (*client.PrepareDomainDelegationResponse, error) {
	return &client.PrepareDomainDelegationResponse{
		NameServers:     []string{"ns1.example.com", "ns2.example.com"},
		DelegationSetId: "test-delegation-set-id",
	}, ctx.Err()
}

func TestComposeUp(t *testing.T) {
	loader := compose.NewLoader(compose.WithPath("../../testdata/testproj/compose.yaml"))
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
	mp := &mockDeployProvider{MockProvider: client.MockProvider{UploadUrl: server.URL + "/"}}
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

func TestGetUnreferencedManagedResources(t *testing.T) {
	t.Run("no services", func(t *testing.T) {
		project := compose.Services{}

		managed, unmanaged := SplitManagedAndUnmanagedServices(project)
		if len(managed) != 0 {
			t.Errorf("Expected 0 managed resources, got %d (%v)", len(managed), managed)
		}

		if len(unmanaged) != 0 {
			t.Errorf("Expected 0 unmanaged resources, got %d (%v)", len(unmanaged), unmanaged)
		}
	})

	t.Run("one service all managed", func(t *testing.T) {
		project := compose.Services{
			"service1": compose.ServiceConfig{
				Extensions: map[string]any{"x-defang-postgres": true},
			},
		}

		managed, unmanaged := SplitManagedAndUnmanagedServices(project)
		if len(managed) != 1 {
			t.Errorf("Expected 1 managed resource, got %d (%v)", len(managed), managed)
		}

		if len(unmanaged) != 0 {
			t.Errorf("Expected 0 unmanaged resource, got %d (%v)", len(unmanaged), unmanaged)
		}
	})

	t.Run("one service unmanaged", func(t *testing.T) {
		project := compose.Services{
			"service1": compose.ServiceConfig{},
		}

		managed, unmanaged := SplitManagedAndUnmanagedServices(project)
		if len(managed) != 0 {
			t.Errorf("Expected 0 managed resource, got %d (%v)", len(managed), managed)
		}

		if len(unmanaged) != 1 {
			t.Errorf("Expected 1 unmanaged resource, got %d (%v)", len(unmanaged), unmanaged)
		}
	})

	t.Run("one service unmanaged, one service managed", func(t *testing.T) {
		project := compose.Services{
			"service1": compose.ServiceConfig{},
			"service2": compose.ServiceConfig{
				Extensions: map[string]any{"x-defang-postgres": true},
			},
		}

		managed, unmanaged := SplitManagedAndUnmanagedServices(project)
		if len(managed) != 1 {
			t.Errorf("Expected 1 managed resource, got %d (%v)", len(managed), managed)
		}

		if len(unmanaged) != 1 {
			t.Errorf("Expected 1 unmanaged resource, got %d (%v)", len(unmanaged), unmanaged)
		}
	})

	t.Run("two service two unmanaged", func(t *testing.T) {
		project := compose.Services{
			"service1": compose.ServiceConfig{},
			"service2": compose.ServiceConfig{},
		}

		managed, unmanaged := SplitManagedAndUnmanagedServices(project)
		if len(managed) != 0 {
			t.Errorf("Expected 0 managed resource, got %d (%v)", len(managed), managed)
		}

		if len(unmanaged) != 2 {
			t.Errorf("Expected 2 unmanaged resource, got %d (%v)", len(unmanaged), unmanaged)
		}
	})

	t.Run("one service two managed", func(t *testing.T) {
		project := compose.Services{
			"service1": compose.ServiceConfig{},
			"service2": compose.ServiceConfig{
				Extensions: map[string]any{"x-defang-postgres": true},
			},
			"service3": compose.ServiceConfig{
				Extensions: map[string]any{"x-defang-redis": true},
			},
		}

		managed, unmanaged := SplitManagedAndUnmanagedServices(project)
		if len(managed) != 2 {
			t.Errorf("Expected 2 managed resource, got %d (%v)", len(managed), managed)
		}
		if len(unmanaged) != 1 {
			t.Errorf("Expected 1 unmanaged resource, got %d (%s)", len(unmanaged), unmanaged)
		}
	})
}

func TestComposeUpStops(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow unit test")
	}

	fabric := client.MockFabricClient{DelegateDomain: "example.com"}
	project := &compose.Project{
		Name: "test-project",
		Services: compose.Services{
			"service1": compose.ServiceConfig{
				Name:       "service1",
				Image:      "test-image",
				DomainName: "test-domain",
			},
		},
	}

	tests := []struct {
		name                  string
		cdStatus              error
		subscribeErr          error
		svcFailed             *defangv1.SubscribeResponse
		wantError             string
		isErrDeploymentFailed bool
	}{
		{"CD task fails", errors.New("not a deployment failure"), nil, nil, "not a deployment failure", false},
		{"CD task fails deployment", client.ErrDeploymentFailed{Message: "EssentialContainerExited: exit code 1"}, nil, nil, "deployment failed: EssentialContainerExited: exit code 1", true},
		{"CD task done, service1 build fails", io.EOF, nil, &defangv1.SubscribeResponse{State: defangv1.ServiceState_BUILD_FAILED, Name: "service1"}, `deployment failed for service "service1": `, true},
		{"CD task done, service1 fails", io.EOF, nil, &defangv1.SubscribeResponse{State: defangv1.ServiceState_DEPLOYMENT_FAILED, Name: "service1"}, `deployment failed for service "service1": `, true},
		{"CD task done, subscription fails", io.EOF, errors.New("subscribe error"), nil, "subscribe error", false},
		{"CD task done, service1 done", io.EOF, nil, &defangv1.SubscribeResponse{State: defangv1.ServiceState_DEPLOYMENT_COMPLETED, Name: "service1"}, "", false},
		{"CD task pending, service1 build fails", nil, nil, &defangv1.SubscribeResponse{State: defangv1.ServiceState_BUILD_FAILED, Name: "service1"}, "context deadline exceeded\ndeployment failed for service \"service1\": ", true},
		{"CD task pending, service1 fails", nil, nil, &defangv1.SubscribeResponse{State: defangv1.ServiceState_DEPLOYMENT_FAILED, Name: "service1"}, "context deadline exceeded\ndeployment failed for service \"service1\": ", true},
		{"CD task pending, subscription fails", nil, errors.New("subscribe error"), nil, "context deadline exceeded\nsubscribe error", false},
		{"CD task pending, service1 done", nil, nil, &defangv1.SubscribeResponse{State: defangv1.ServiceState_DEPLOYMENT_COMPLETED, Name: "service1"}, "context deadline exceeded", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
			t.Cleanup(cancel)

			provider := &mockDeployProvider{
				deploymentStatus: tt.cdStatus,
			}

			resp, project, err := ComposeUp(ctx, project, fabric, provider, compose.UploadModeDigest, defangv1.DeploymentMode_MODE_UNSPECIFIED)
			if err != nil {
				t.Fatalf("ComposeUp() failed: %v", err)
			}
			if tt.svcFailed != nil || tt.subscribeErr != nil {
				timer := time.AfterFunc(time.Second, func() { provider.subscribeStream.Send(tt.svcFailed, tt.subscribeErr) })
				t.Cleanup(func() { timer.Stop() })
			}
			err = TailAndMonitor(ctx, project, provider, -1, TailOptions{Deployment: resp.Etag})
			if err != nil {
				if err.Error() != tt.wantError {
					t.Errorf("expected error: %v, got: %v", tt.wantError, err)
				}
			} else if tt.wantError != "" {
				t.Errorf("expected error: %v, got: nil", tt.wantError)
			}
			var errDeploymentFailed client.ErrDeploymentFailed
			if errors.As(err, &errDeploymentFailed) != tt.isErrDeploymentFailed {
				t.Errorf("expected ErrDeploymentFailed: %v, got: %v", tt.isErrDeploymentFailed, err)
			}
		})
	}
}
