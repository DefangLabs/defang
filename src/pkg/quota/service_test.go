package quota

import (
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/aws/smithy-go/ptr"
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
			service: &types.ServiceConfig{Name: "test", Image: "asdf", Ports: []types.ServicePortConfig{{Target: 53, Mode: compose.Mode_INGRESS, Protocol: compose.Protocol_UDP}}},
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
			wantErr: "invalid healthcheck: timeout must be less than the interval",
		},
		{
			name: "invalid CMD healthcheck",
			service: &types.ServiceConfig{
				Name:  "test",
				Image: "asdf",
				Ports: []types.ServicePortConfig{{Target: 80, Mode: compose.Mode_INGRESS, Protocol: compose.Protocol_HTTP}},
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
				Ports: []types.ServicePortConfig{{Target: 80, Mode: compose.Mode_INGRESS, Protocol: compose.Protocol_HTTP}},
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
				Ports: []types.ServicePortConfig{{Target: 80, Mode: compose.Mode_INGRESS, Protocol: compose.Protocol_HTTP}},
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
				Ports: []types.ServicePortConfig{{Target: 80, Mode: compose.Mode_INGRESS, Protocol: compose.Protocol_HTTP}},
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
				Ports: []types.ServicePortConfig{{Target: 80, Mode: compose.Mode_INGRESS, Protocol: compose.Protocol_HTTP}},
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
				Ports: []types.ServicePortConfig{{Target: 80, Mode: compose.Mode_INGRESS, Protocol: compose.Protocol_HTTP}},
				HealthCheck: &types.HealthCheckConfig{
					Test: []string{"CMD", "curl", "http://localhost"},
				},
				Deploy: &types.DeployConfig{
					Resources: types.Resources{
						Reservations: &types.Resource{
							NanoCPUs:    1,
							MemoryBytes: compose.MiB * 1024,
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

	byoc := Quotas{
		ServiceQuotas: ServiceQuotas{
			Cpus:       16,
			Gpus:       8,
			MemoryMiB:  65536,
			Replicas:   16,
			ShmSizeMiB: 30720,
		},
		Services: 40,
		Ingress:  10,
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := byoc.ValidateService(tt.service); err != nil && err.Error() != tt.wantErr {
				t.Errorf("Byoc.ValidateService() = %q, want %q", err, tt.wantErr)
			} else if err == nil && tt.wantErr != "" {
				t.Errorf("Byoc.ValidateService() = nil, want %q", tt.wantErr)
			}
		})
	}
}

func TestValidateQuotas(t *testing.T) {
	tests := []struct {
		name    string
		service *types.ServiceConfig
		wantErr string
	}{
		{
			name:    "shm size exceeds quota",
			service: &types.ServiceConfig{Name: "test", Build: &types.BuildConfig{Context: ".", ShmSize: 30721 * compose.MiB}},
			wantErr: "build.shm_size 30721 MiB exceeds quota 30720 MiB",
		},
		{
			name: "too many replicas",
			service: &types.ServiceConfig{
				Name:  "test",
				Image: "asdf",
				Deploy: &types.DeployConfig{
					Replicas: ptr.Int(100),
				},
			},
			wantErr: "replicas exceeds quota (max 16)",
		},
		{
			name: "too many CPUs",
			service: &types.ServiceConfig{
				Name:  "test",
				Image: "asdf",
				Deploy: &types.DeployConfig{
					Resources: types.Resources{
						Reservations: &types.Resource{
							NanoCPUs: 100,
						},
					},
				},
			},
			wantErr: "cpus exceeds quota (max 16 vCPU)",
		},
		{
			name: "negative cpus",
			service: &types.ServiceConfig{
				Name:  "test",
				Image: "asdf",
				Deploy: &types.DeployConfig{
					Resources: types.Resources{
						Reservations: &types.Resource{
							NanoCPUs: -1,
						},
					},
				},
			},
			wantErr: "cpus exceeds quota (max 16 vCPU)",
		},
		{
			name: "too much memory",
			service: &types.ServiceConfig{
				Name:  "test",
				Image: "asdf",
				Deploy: &types.DeployConfig{
					Resources: types.Resources{
						Reservations: &types.Resource{
							MemoryBytes: compose.MiB * 200000,
						},
					},
				},
			},
			wantErr: "memory 200000 MiB exceeds quota 65536 MiB",
		},
		{
			name: "negative memory",
			service: &types.ServiceConfig{
				Name:  "test",
				Image: "asdf",
				Deploy: &types.DeployConfig{
					Resources: types.Resources{
						Reservations: &types.Resource{
							MemoryBytes: compose.MiB * -1,
						},
					},
				},
			},
			wantErr: "memory -1 MiB exceeds quota 65536 MiB",
		},
		{
			name: "only GPU",
			service: &types.ServiceConfig{
				Name:  "test",
				Image: "asdf",
				Deploy: &types.DeployConfig{
					Resources: types.Resources{
						Reservations: &types.Resource{
							Devices: []types.DeviceRequest{
								{Capabilities: []string{"tpu"}},
							},
						},
					},
				},
			},
			wantErr: "only GPU devices are supported",
		},
		{
			name: "only nvidia GPU",
			service: &types.ServiceConfig{
				Name:  "test",
				Image: "asdf",
				Deploy: &types.DeployConfig{
					Resources: types.Resources{
						Reservations: &types.Resource{
							Devices: []types.DeviceRequest{
								{Capabilities: []string{"gpu"}, Driver: "amd"},
							},
						},
					},
				},
			},
			wantErr: "only nvidia GPU devices are supported",
		},
		{
			name: "too many GPUs",
			service: &types.ServiceConfig{
				Name:  "test",
				Image: "asdf",
				Deploy: &types.DeployConfig{
					Resources: types.Resources{
						Reservations: &types.Resource{
							Devices: []types.DeviceRequest{
								{Capabilities: []string{"gpu"}, Driver: "nvidia", Count: 100},
							},
						},
					},
				},
			},
			wantErr: "gpu count 100 exceeds quota 8",
		},
		{
			name: "valid service",
			service: &types.ServiceConfig{
				Name:  "test",
				Image: "asdf",
				Ports: []types.ServicePortConfig{{Target: 80, Mode: compose.Mode_INGRESS, Protocol: compose.Protocol_HTTP}},
				HealthCheck: &types.HealthCheckConfig{
					Test: []string{"CMD", "curl", "http://localhost"},
				},
				Deploy: &types.DeployConfig{
					Resources: types.Resources{
						Reservations: &types.Resource{
							NanoCPUs:    1,
							MemoryBytes: compose.MiB * 1024,
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

	byoc := Quotas{
		ServiceQuotas: ServiceQuotas{
			Cpus:       16,
			Gpus:       8,
			MemoryMiB:  65536,
			Replicas:   16,
			ShmSizeMiB: 30720,
		},
		Services: 40,
		Ingress:  10,
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := byoc.ValidateQuotas(tt.service); err != nil && err.Error() != tt.wantErr {
				t.Errorf("Byoc.ValidateQuotas() = %q, want %q", err, tt.wantErr)
			} else if err == nil && tt.wantErr != "" {
				t.Errorf("Byoc.ValidateQuotas() = nil, want %q", tt.wantErr)
			}
		})
	}
}

func duration(d types.Duration) *types.Duration {
	return &d
}
