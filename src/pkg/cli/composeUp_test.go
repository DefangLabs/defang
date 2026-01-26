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

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/dryrun"
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/DefangLabs/defang/src/pkg/stacks"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/stretchr/testify/require"
)

type mockDeployProvider struct {
	client.MockProvider
	deploymentStatus  error
	subscribeStream   *client.MockWaitStream[defangv1.SubscribeResponse]
	tailStream        *client.MockWaitStream[defangv1.TailResponse]
	prevProjectUpdate *defangv1.ProjectUpdate
}

func (d mockDeployProvider) Deploy(ctx context.Context, req *client.DeployRequest) (*defangv1.DeployResponse, error) {
	return d.Preview(ctx, req)
}

func (mockDeployProvider) Preview(ctx context.Context, req *client.DeployRequest) (*defangv1.DeployResponse, error) {
	if len(req.Compose) == 0 {
		return nil, errors.New("DeployRequest needs Compose")
	}

	project, err := compose.LoadFromContent(ctx, req.Compose, "")
	if err != nil {
		return nil, err
	}
	if project.Name == "" {
		return nil, errors.New("project name is required")
	}

	etag := types.NewEtag()
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

func (m mockDeployProvider) GetProjectUpdate(ctx context.Context, projectName string) (*defangv1.ProjectUpdate, error) {
	return m.prevProjectUpdate, ctx.Err()
}

func (m mockDeployProvider) GetDeploymentStatus(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return context.Cause(ctx)
	default:
		return m.deploymentStatus
	}
}

func (mockDeployProvider) AccountInfo(ctx context.Context) (*client.AccountInfo, error) {
	return &client.AccountInfo{}, ctx.Err()
}

func (mockDeployProvider) PrepareDomainDelegation(ctx context.Context, req client.PrepareDomainDelegationRequest) (*client.PrepareDomainDelegationResponse, error) {
	return &client.PrepareDomainDelegationResponse{
		NameServers:     []string{"ns1.example.com", "ns2.example.com"},
		DelegationSetId: "test-delegation-set-id",
	}, ctx.Err()
}

func TestComposeUp(t *testing.T) {
	loader := compose.NewLoader(compose.WithPath("../../testdata/testproj/compose.yaml"))
	proj, err := loader.LoadProject(t.Context())
	if err != nil {
		t.Fatalf("LoadProject() failed: %v", err)
	}

	gotContext := atomic.Bool{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("ComposeStart() failed: expected PUT request, got: %s", r.Method)
		}
		gotContext.Store(true)
		w.WriteHeader(http.StatusOK) // return 200 OK same as S3
	}))
	t.Cleanup(server.Close)

	mc := client.MockFabricClient{DelegateDomain: "example.com"}
	mp := &mockDeployProvider{MockProvider: client.MockProvider{UploadUrl: server.URL + "/"}}
	stack := &stacks.Parameters{
		Provider: client.ProviderDefang,
	}

	t.Run("happy path", func(t *testing.T) {
		d, project, err := ComposeUp(t.Context(), mc, mp, stack, ComposeUpParams{
			Mode:       modes.ModeAffordable,
			Project:    proj,
			UploadMode: compose.UploadModeDigest,
		})
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
	})

	t.Run("no downgrade from HA to affordable", func(t *testing.T) {
		mp.prevProjectUpdate = &defangv1.ProjectUpdate{
			Mode: defangv1.DeploymentMode_PRODUCTION,
		}
		_, _, err = ComposeUp(t.Context(), mc, mp, stack, ComposeUpParams{
			Mode:       modes.ModeAffordable,
			Project:    proj,
			UploadMode: compose.UploadModeDigest,
		})
		require.ErrorContains(t, err, "downgrade deployment mode from HIGH_AVAILABILITY to AFFORDABLE")
	})
}

func TestSplitManagedAndUnmanagedServices(t *testing.T) {
	t.Run("no services", func(t *testing.T) {
		project := compose.Services{}

		managed, unmanaged := splitManagedAndUnmanagedServices(project)
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

		managed, unmanaged := splitManagedAndUnmanagedServices(project)
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

		managed, unmanaged := splitManagedAndUnmanagedServices(project)
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

		managed, unmanaged := splitManagedAndUnmanagedServices(project)
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

		managed, unmanaged := splitManagedAndUnmanagedServices(project)
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

		managed, unmanaged := splitManagedAndUnmanagedServices(project)
		if len(managed) != 2 {
			t.Errorf("Expected 2 managed resource, got %d (%v)", len(managed), managed)
		}
		if len(unmanaged) != 1 {
			t.Errorf("Expected 1 unmanaged resource, got %d (%s)", len(unmanaged), unmanaged)
		}
	})

	t.Run("release task with restart: 'no'", func(t *testing.T) {
		project := compose.Services{
			"service1": compose.ServiceConfig{
				Restart: "no",
			},
		}

		managed, unmanaged := splitManagedAndUnmanagedServices(project)
		if len(managed) != 1 {
			t.Errorf("Expected 1 managed resource, got %d (%v)", len(managed), managed)
		}

		if len(unmanaged) != 0 {
			t.Errorf("Expected 0 unmanaged resource, got %d (%v)", len(unmanaged), unmanaged)
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
			ctx, cancel := context.WithTimeout(t.Context(), time.Second*5)
			t.Cleanup(cancel)

			provider := &mockDeployProvider{
				deploymentStatus: tt.cdStatus,
			}

			stack := &stacks.Parameters{
				Provider: client.ProviderDefang,
			}

			resp, project, err := ComposeUp(ctx, fabric, provider, stack, ComposeUpParams{
				Mode:       modes.ModeUnspecified,
				Project:    project,
				UploadMode: compose.UploadModeDigest,
			})
			if err != nil {
				t.Fatalf("ComposeUp() failed: %v", err)
			}
			if tt.svcFailed != nil || tt.subscribeErr != nil {
				timer := time.AfterFunc(time.Second, func() { provider.subscribeStream.Send(tt.svcFailed, tt.subscribeErr) })
				t.Cleanup(func() { timer.Stop() })
			}
			_, err = TailAndMonitor(ctx, project, provider, -1, TailOptions{Deployment: resp.Etag})
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

func TestComposeConfigWithoutLogin(t *testing.T) {
	fabric := client.MockFabricClient{}
	provider := &client.PlaygroundProvider{FabricClient: fabric}

	project := &compose.Project{}
	stack := &stacks.Parameters{}

	_, _, err := ComposeUp(t.Context(), fabric, provider, stack, ComposeUpParams{
		Mode:       modes.ModeUnspecified,
		Project:    project,
		UploadMode: compose.UploadModeIgnore,
	})
	if !errors.Is(err, dryrun.ErrDryRun) {
		t.Fatalf("ComposeUp() failed: %v", err)
	}
}

func Test_checkDeploymentMode(t *testing.T) {
	// previous deployment mode | new mode          | behavior:
	// -------------------------|-------------------|-----------------------
	// any                      | unspecified       | previous mode
	// affordable               | affordable        | nop, use affordable
	// affordable               | balanced          | new mode: balanced
	// affordable               | high-availability | new mode: high-availability
	// balanced                 | affordable        | warn, use new mode: affordable
	// balanced                 | balanced          | nop, use balanced
	// balanced                 | high-availability | new mode: high-availability
	// high-availability        | affordable        | error
	// high-availability        | balanced          | warn, use balanced
	// high-availability        | high-availability | nop, use high-availability
	tests := []struct {
		prevMode modes.Mode
		newMode  modes.Mode
		wantMode modes.Mode
		wantErr  bool
	}{
		{modes.ModeUnspecified, modes.ModeUnspecified, modes.ModeUnspecified, false},
		{modes.ModeUnspecified, modes.ModeAffordable, modes.ModeAffordable, false},
		{modes.ModeUnspecified, modes.ModeBalanced, modes.ModeBalanced, false},
		{modes.ModeUnspecified, modes.ModeHighAvailability, modes.ModeHighAvailability, false},
		{modes.ModeAffordable, modes.ModeUnspecified, modes.ModeAffordable, false},
		{modes.ModeAffordable, modes.ModeAffordable, modes.ModeAffordable, false},
		{modes.ModeAffordable, modes.ModeBalanced, modes.ModeBalanced, false},
		{modes.ModeAffordable, modes.ModeHighAvailability, modes.ModeHighAvailability, false},
		{modes.ModeBalanced, modes.ModeUnspecified, modes.ModeBalanced, false},
		{modes.ModeBalanced, modes.ModeAffordable, modes.ModeAffordable, false},
		{modes.ModeBalanced, modes.ModeBalanced, modes.ModeBalanced, false},
		{modes.ModeBalanced, modes.ModeHighAvailability, modes.ModeHighAvailability, false},
		{modes.ModeHighAvailability, modes.ModeUnspecified, modes.ModeHighAvailability, false},
		{modes.ModeHighAvailability, modes.ModeAffordable, modes.ModeAffordable, true},
		{modes.ModeHighAvailability, modes.ModeBalanced, modes.ModeBalanced, false},
		{modes.ModeHighAvailability, modes.ModeHighAvailability, modes.ModeHighAvailability, false},
	}

	for _, tt := range tests {
		t.Run(tt.prevMode.String()+"->"+tt.newMode.String(), func(t *testing.T) {
			gotMode, err := checkDeploymentMode(tt.prevMode, tt.newMode)
			if (err != nil) != tt.wantErr {
				t.Fatalf("checkDeploymentMode() error = %v, wantErr %v", err, tt.wantErr)
			}
			if gotMode != tt.wantMode {
				t.Errorf("checkDeploymentMode() gotMode = %v, want %v", gotMode, tt.wantMode)
			}
		})
	}
}
