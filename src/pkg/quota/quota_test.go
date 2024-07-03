package quota

import (
	"testing"

	"github.com/aws/smithy-go/ptr"
	compose "github.com/compose-spec/compose-go/v2/types"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		service *compose.ServiceConfig
		wantErr string
	}{
		{
			name:    "empty service",
			service: &compose.ServiceConfig{},
			wantErr: "service name is required",
		},
		{
			name:    "no image, no build",
			service: &compose.ServiceConfig{Name: "test"},
			wantErr: "missing image or build",
		},
		{
			name:    "empty build",
			service: &compose.ServiceConfig{Name: "test", Build: &compose.BuildConfig{}},
			wantErr: "build.context is required",
		},
		{
			name:    "shm size exceeds quota",
			service: &compose.ServiceConfig{Name: "test", Build: &compose.BuildConfig{Context: ".", ShmSize: 30721 * MiB}},
			wantErr: "build.shm_size exceeds quota (max 30720 MiB)",
		},
		{
			name:    "port 0 out of range",
			service: &compose.ServiceConfig{Name: "test", Image: "asdf", Ports: []compose.ServicePortConfig{{Target: 0}}},
			wantErr: "port 0 is out of range",
		},
		{
			name:    "port out of range",
			service: &compose.ServiceConfig{Name: "test", Image: "asdf", Ports: []compose.ServicePortConfig{{Target: 33333}}},
			wantErr: "port 33333 is out of range",
		},
		{
			name:    "ingress with UDP",
			service: &compose.ServiceConfig{Name: "test", Image: "asdf", Ports: []compose.ServicePortConfig{{Target: 53, Mode: Mode_INGRESS, Protocol: Protocol_UDP}}},
			wantErr: "mode:INGRESS is not supported by protocol:udp",
		},
		{
			name:    "ingress with UDP",
			service: &compose.ServiceConfig{Name: "test", Image: "asdf", Ports: []compose.ServicePortConfig{{Target: 80, Mode: Mode_INGRESS, Protocol: Protocol_TCP}}},
			wantErr: "mode:INGRESS is not supported by protocol:tcp",
		},
		{
			name: "invalid healthcheck interval",
			service: &compose.ServiceConfig{
				Name:  "test",
				Image: "asdf",
				HealthCheck: &compose.HealthCheckConfig{
					Test:     []string{"CMD-SHELL", "echo 1"},
					Interval: duration(1),
					Timeout:  duration(2),
				},
			},
			wantErr: "invalid healthcheck: timeout must be less than the interval",
		},
		{
			name: "invalid CMD healthcheck",
			service: &compose.ServiceConfig{
				Name:  "test",
				Image: "asdf",
				Ports: []compose.ServicePortConfig{{Target: 80, Mode: Mode_INGRESS, Protocol: Protocol_HTTP}},
				HealthCheck: &compose.HealthCheckConfig{
					Test: []string{"CMD", "echo 1"},
				},
			},
			wantErr: "invalid CMD healthcheck: expected a command and URL",
		},
		{
			name: "CMD without curl or wget",
			service: &compose.ServiceConfig{
				Name:  "test",
				Image: "asdf",
				Ports: []compose.ServicePortConfig{{Target: 80, Mode: Mode_INGRESS, Protocol: Protocol_HTTP}},
				HealthCheck: &compose.HealthCheckConfig{
					Test: []string{"CMD", "echo", "1"},
				},
			},
			wantErr: "invalid CMD healthcheck: expected curl or wget",
		},
		{
			name: "CMD without HTTP URL",
			service: &compose.ServiceConfig{
				Name:  "test",
				Image: "asdf",
				Ports: []compose.ServicePortConfig{{Target: 80, Mode: Mode_INGRESS, Protocol: Protocol_HTTP}},
				HealthCheck: &compose.HealthCheckConfig{
					Test: []string{"CMD", "curl", "1"},
				},
			},
			wantErr: "invalid CMD healthcheck; missing HTTP URL",
		},
		{
			name: "NONE with arguments",
			service: &compose.ServiceConfig{
				Name:  "test",
				Image: "asdf",
				HealthCheck: &compose.HealthCheckConfig{
					Test: []string{"NONE", "echo", "1"},
				},
			},
			wantErr: "invalid NONE healthcheck; expected no arguments",
		},
		{
			name: "CMD-SHELL with ingress",
			service: &compose.ServiceConfig{
				Name:  "test",
				Image: "asdf",
				Ports: []compose.ServicePortConfig{{Target: 80, Mode: Mode_INGRESS, Protocol: Protocol_HTTP}},
				HealthCheck: &compose.HealthCheckConfig{
					Test: []string{"CMD-SHELL", "echo 1"},
				},
			},
			wantErr: "ingress port requires a CMD healthcheck",
		},
		{
			name: "NONE with ingress",
			service: &compose.ServiceConfig{
				Name:  "test",
				Image: "asdf",
				Ports: []compose.ServicePortConfig{{Target: 80, Mode: Mode_INGRESS, Protocol: Protocol_HTTP}},
				HealthCheck: &compose.HealthCheckConfig{
					Test: []string{"NONE"},
				},
			},
			wantErr: "ingress port requires a CMD healthcheck",
		},
		{
			name: "unsupported healthcheck test",
			service: &compose.ServiceConfig{
				Name:  "test",
				Image: "asdf",
				HealthCheck: &compose.HealthCheckConfig{
					Test: []string{"BLAH"},
				},
			},
			wantErr: "unsupported healthcheck: [BLAH]",
		},
		{
			name: "too many replicas",
			service: &compose.ServiceConfig{
				Name:  "test",
				Image: "asdf",
				Deploy: &compose.DeployConfig{
					Replicas: ptr.Int(100),
				},
			},
			wantErr: "replicas exceeds quota (max 16)",
		},
		{
			name: "too many CPUs",
			service: &compose.ServiceConfig{
				Name:  "test",
				Image: "asdf",
				Deploy: &compose.DeployConfig{
					Resources: compose.Resources{
						Reservations: &compose.Resource{
							NanoCPUs: 100,
						},
					},
				},
			},
			wantErr: "cpus exceeds quota (max 16 vCPU)",
		},
		{
			name: "negative cpus",
			service: &compose.ServiceConfig{
				Name:  "test",
				Image: "asdf",
				Deploy: &compose.DeployConfig{
					Resources: compose.Resources{
						Reservations: &compose.Resource{
							NanoCPUs: -1,
						},
					},
				},
			},
			wantErr: "cpus exceeds quota (max 16 vCPU)",
		},
		{
			name: "too much memory",
			service: &compose.ServiceConfig{
				Name:  "test",
				Image: "asdf",
				Deploy: &compose.DeployConfig{
					Resources: compose.Resources{
						Reservations: &compose.Resource{
							MemoryBytes: MiB * 200000,
						},
					},
				},
			},
			wantErr: "memory exceeds quota (max 65536 MiB)",
		},
		{
			name: "negative memory",
			service: &compose.ServiceConfig{
				Name:  "test",
				Image: "asdf",
				Deploy: &compose.DeployConfig{
					Resources: compose.Resources{
						Reservations: &compose.Resource{
							MemoryBytes: MiB * -1,
						},
					},
				},
			},
			wantErr: "memory exceeds quota (max 65536 MiB)",
		},
		{
			name: "only GPU",
			service: &compose.ServiceConfig{
				Name:  "test",
				Image: "asdf",
				Deploy: &compose.DeployConfig{
					Resources: compose.Resources{
						Reservations: &compose.Resource{
							Devices: []compose.DeviceRequest{
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
			service: &compose.ServiceConfig{
				Name:  "test",
				Image: "asdf",
				Deploy: &compose.DeployConfig{
					Resources: compose.Resources{
						Reservations: &compose.Resource{
							Devices: []compose.DeviceRequest{
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
			service: &compose.ServiceConfig{
				Name:  "test",
				Image: "asdf",
				Deploy: &compose.DeployConfig{
					Resources: compose.Resources{
						Reservations: &compose.Resource{
							Devices: []compose.DeviceRequest{
								{Capabilities: []string{"gpu"}, Driver: "nvidia", Count: 100},
							},
						},
					},
				},
			},
			wantErr: "gpu count exceeds quota (max 8)",
		},
		{
			name: "valid service",
			service: &compose.ServiceConfig{
				Name:  "test",
				Image: "asdf",
				Ports: []compose.ServicePortConfig{{Target: 80, Mode: Mode_INGRESS, Protocol: Protocol_HTTP}},
				HealthCheck: &compose.HealthCheckConfig{
					Test: []string{"CMD", "curl", "http://localhost"},
				},
				Deploy: &compose.DeployConfig{
					Resources: compose.Resources{
						Reservations: &compose.Resource{
							NanoCPUs:    1,
							MemoryBytes: MiB * 1024,
							Devices: []compose.DeviceRequest{
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
		Cpus:       16,
		Gpus:       8,
		MemoryMiB:  65536,
		Replicas:   16,
		Services:   40,
		ShmSizeMiB: 30720,
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := byoc.Validate(tt.service); err != nil && err.Error() != tt.wantErr {
				t.Errorf("Byoc.Validate() = %v, want %v", err, tt.wantErr)
			} else if err == nil && tt.wantErr != "" {
				t.Errorf("Byoc.Validate() = nil, want %v", tt.wantErr)
			}
		})
	}
}

func duration(d compose.Duration) *compose.Duration {
	return &d
}
