package compose

import (
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
)

func TestValidateService(t *testing.T) {
	tests := []struct {
		name    string
		service *types.ServiceConfig
		wantErr string
	}{
		{
			name:    "empty service",
			service: &types.ServiceConfig{},
			wantErr: "service name is required",
		},
		{
			name:    "no image, no build",
			service: &types.ServiceConfig{Name: "test"},
			wantErr: "missing image or build",
		},
		{
			name:    "empty build",
			service: &types.ServiceConfig{Name: "test", Build: &types.BuildConfig{}},
			wantErr: "build.context is required",
		},
		{
			name:    "port 0 out of range",
			service: &types.ServiceConfig{Name: "test", Image: "asdf", Ports: []types.ServicePortConfig{{Target: 0}}},
			wantErr: "port 0 is out of range",
		},
		{
			name:    "port out of range",
			service: &types.ServiceConfig{Name: "test", Image: "asdf", Ports: []types.ServicePortConfig{{Target: 33333}}},
			wantErr: "port 33333 is out of range",
		},
		{
			name:    "ingress with UDP",
			service: &types.ServiceConfig{Name: "test", Image: "asdf", Ports: []types.ServicePortConfig{{Target: 53, Mode: "ingress", Protocol: "udp"}}},
			wantErr: "`mode: ingress` is not supported by `protocol: udp`",
		},
		{
			name: "invalid healthcheck interval",
			service: &types.ServiceConfig{
				Name:  "test",
				Image: "asdf",
				HealthCheck: &types.HealthCheckConfig{
					Test:     []string{"CMD-SHELL", "echo 1"},
					Interval: duration(1),
					Timeout:  duration(2),
				},
			},
		},
		{
			name: "invalid CMD healthcheck",
			service: &types.ServiceConfig{
				Name:  "test",
				Image: "asdf",
				Ports: []types.ServicePortConfig{{Target: 80, Mode: "ingress", Protocol: "http"}},
				HealthCheck: &types.HealthCheckConfig{
					Test: []string{"CMD", "echo 1"},
				},
			},
			wantErr: "invalid healthcheck: ingress ports require an HTTP healthcheck on `localhost`, see https://s.defang.io/healthchecks",
		},
		{
			name: "CMD without curl or wget",
			service: &types.ServiceConfig{
				Name:  "test",
				Image: "asdf",
				Ports: []types.ServicePortConfig{{Target: 80, Mode: "ingress", Protocol: "http"}},
				HealthCheck: &types.HealthCheckConfig{
					Test: []string{"CMD", "echo", "1"},
				},
			},
			wantErr: "invalid healthcheck: ingress ports require an HTTP healthcheck on `localhost`, see https://s.defang.io/healthchecks",
		},
		{
			name: "CMD without HTTP URL",
			service: &types.ServiceConfig{
				Name:  "test",
				Image: "asdf",
				Ports: []types.ServicePortConfig{{Target: 80, Mode: "ingress", Protocol: "http"}},
				HealthCheck: &types.HealthCheckConfig{
					Test: []string{"CMD", "curl", "1"},
				},
			},
			wantErr: "invalid healthcheck: ingress ports require an HTTP healthcheck on `localhost`, see https://s.defang.io/healthchecks",
		},
		{
			name: "NONE with arguments",
			service: &types.ServiceConfig{
				Name:  "test",
				Image: "asdf",
				HealthCheck: &types.HealthCheckConfig{
					Test: []string{"NONE", "echo", "1"},
				},
			},
			// wantErr: "invalid NONE healthcheck; expected no arguments",
		},
		{
			name: "CMD-SHELL with ingress",
			service: &types.ServiceConfig{
				Name:  "test",
				Image: "asdf",
				Ports: []types.ServicePortConfig{{Target: 80, Mode: "ingress", Protocol: "http"}},
				HealthCheck: &types.HealthCheckConfig{
					Test: []string{"CMD-SHELL", "echo 1"},
				},
			},
			wantErr: "invalid healthcheck: ingress ports require an HTTP healthcheck on `localhost`, see https://s.defang.io/healthchecks",
		},
		{
			name: "NONE with ingress",
			service: &types.ServiceConfig{
				Name:  "test",
				Image: "asdf",
				Ports: []types.ServicePortConfig{{Target: 80, Mode: "ingress", Protocol: "http"}},
				HealthCheck: &types.HealthCheckConfig{
					Test: []string{"NONE"},
				},
			},
			wantErr: "invalid healthcheck: ingress ports require a CMD or CMD-SHELL healthcheck, see https://s.defang.io/healthchecks",
		},
		{
			name: "unsupported healthcheck test",
			service: &types.ServiceConfig{
				Name:  "test",
				Image: "asdf",
				HealthCheck: &types.HealthCheckConfig{
					Test: []string{"BLAH"},
				},
			},
			wantErr: "unsupported healthcheck: [BLAH]",
		},
		{
			name: "valid service",
			service: &types.ServiceConfig{
				Name:  "test",
				Image: "asdf",
				Ports: []types.ServicePortConfig{{Target: 80, Mode: "ingress", Protocol: "http"}},
				HealthCheck: &types.HealthCheckConfig{
					Test: []string{"CMD", "curl", "http://localhost"},
				},
				Deploy: &types.DeployConfig{
					Resources: types.Resources{
						Reservations: &types.Resource{
							NanoCPUs:    1,
							MemoryBytes: MiB * 1024,
							Devices: []types.DeviceRequest{
								{
									Capabilities: []string{"gpu"},
									Driver:       "nvidia",
									Count:        1,
								},
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateService(tt.service); err != nil && err.Error() != tt.wantErr {
				t.Errorf("ValidateService() = %q, want %q", err, tt.wantErr)
			} else if err == nil && tt.wantErr != "" {
				t.Errorf("ValidateService() = nil, want %q", tt.wantErr)
			}
		})
	}
}

func duration(d types.Duration) *types.Duration {
	return &d
}
