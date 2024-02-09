package quota

import (
	"testing"

	v1 "github.com/defang-io/defang/src/protos/io/defang/v1"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		service *v1.Service
		wantErr string
	}{
		{
			name:    "empty service",
			service: &v1.Service{},
			wantErr: "service name is required",
		},
		{
			name:    "no image, no build",
			service: &v1.Service{Name: "test"},
			wantErr: "missing image or build",
		},
		{
			name:    "empty build",
			service: &v1.Service{Name: "test", Build: &v1.Build{}},
			wantErr: "build.context is required",
		},
		{
			name:    "shm size exceeds quota",
			service: &v1.Service{Name: "test", Build: &v1.Build{Context: ".", ShmSize: 30721}},
			wantErr: "build.shm_size exceeds quota (max 30720 MiB)",
		},
		{
			name:    "port 0 out of range",
			service: &v1.Service{Name: "test", Image: "asdf", Ports: []*v1.Port{{Target: 0}}},
			wantErr: "port 0 is out of range",
		},
		{
			name:    "port out of range",
			service: &v1.Service{Name: "test", Image: "asdf", Ports: []*v1.Port{{Target: 33333}}},
			wantErr: "port 33333 is out of range",
		},
		{
			name:    "ingress with UDP",
			service: &v1.Service{Name: "test", Image: "asdf", Ports: []*v1.Port{{Target: 53, Mode: v1.Mode_INGRESS, Protocol: v1.Protocol_UDP}}},
			wantErr: "mode:INGRESS is not supported by protocol:UDP",
		},
		{
			name:    "ingress with UDP",
			service: &v1.Service{Name: "test", Image: "asdf", Ports: []*v1.Port{{Target: 80, Mode: v1.Mode_INGRESS, Protocol: v1.Protocol_TCP}}},
			wantErr: "mode:INGRESS is not supported by protocol:TCP",
		},
		{
			name: "invalid healthcheck interval",
			service: &v1.Service{
				Name:  "test",
				Image: "asdf",
				Healthcheck: &v1.HealthCheck{
					Test:     []string{"CMD-SHELL", "echo 1"},
					Interval: 1,
					Timeout:  2,
				},
			},
			wantErr: "invalid healthcheck: timeout must be less than the interval",
		},
		{
			name: "invalid CMD healthcheck",
			service: &v1.Service{
				Name:  "test",
				Image: "asdf",
				Ports: []*v1.Port{{Target: 80, Mode: v1.Mode_INGRESS, Protocol: v1.Protocol_HTTP}},
				Healthcheck: &v1.HealthCheck{
					Test: []string{"CMD", "echo 1"},
				},
			},
			wantErr: "invalid CMD healthcheck: expected a command and URL",
		},
		{
			name: "CMD without curl or wget",
			service: &v1.Service{
				Name:  "test",
				Image: "asdf",
				Ports: []*v1.Port{{Target: 80, Mode: v1.Mode_INGRESS, Protocol: v1.Protocol_HTTP}},
				Healthcheck: &v1.HealthCheck{
					Test: []string{"CMD", "echo", "1"},
				},
			},
			wantErr: "invalid CMD healthcheck: expected curl or wget",
		},
		{
			name: "CMD without HTTP URL",
			service: &v1.Service{
				Name:  "test",
				Image: "asdf",
				Ports: []*v1.Port{{Target: 80, Mode: v1.Mode_INGRESS, Protocol: v1.Protocol_HTTP}},
				Healthcheck: &v1.HealthCheck{
					Test: []string{"CMD", "curl", "1"},
				},
			},
			wantErr: "invalid CMD healthcheck; missing HTTP URL",
		},
		{
			name: "NONE with arguments",
			service: &v1.Service{
				Name:  "test",
				Image: "asdf",
				Healthcheck: &v1.HealthCheck{
					Test: []string{"NONE", "echo", "1"},
				},
			},
			wantErr: "invalid NONE healthcheck; expected no arguments",
		},
		{
			name: "CMD-SHELL with ingress",
			service: &v1.Service{
				Name:  "test",
				Image: "asdf",
				Ports: []*v1.Port{{Target: 80, Mode: v1.Mode_INGRESS, Protocol: v1.Protocol_HTTP}},
				Healthcheck: &v1.HealthCheck{
					Test: []string{"CMD-SHELL", "echo 1"},
				},
			},
			wantErr: "ingress port requires a CMD healthcheck",
		},
		{
			name: "NONE with ingress",
			service: &v1.Service{
				Name:  "test",
				Image: "asdf",
				Ports: []*v1.Port{{Target: 80, Mode: v1.Mode_INGRESS, Protocol: v1.Protocol_HTTP}},
				Healthcheck: &v1.HealthCheck{
					Test: []string{"NONE"},
				},
			},
			wantErr: "ingress port requires a CMD healthcheck",
		},
		{
			name: "unsupported healthcheck test",
			service: &v1.Service{
				Name:  "test",
				Image: "asdf",
				Healthcheck: &v1.HealthCheck{
					Test: []string{"BLAH"},
				},
			},
			wantErr: "unsupported healthcheck: [BLAH]",
		},
		{
			name: "too many replicas",
			service: &v1.Service{
				Name:  "test",
				Image: "asdf",
				Deploy: &v1.Deploy{
					Replicas: 100,
				},
			},
			wantErr: "replicas exceeds quota (max 16)",
		},
		{
			name: "too many CPUs",
			service: &v1.Service{
				Name:  "test",
				Image: "asdf",
				Deploy: &v1.Deploy{
					Resources: &v1.Resources{
						Reservations: &v1.Resource{
							Cpus: 100,
						},
					},
				},
			},
			wantErr: "cpus exceeds quota (max 16 vCPU)",
		},
		{
			name: "negative cpus",
			service: &v1.Service{
				Name:  "test",
				Image: "asdf",
				Deploy: &v1.Deploy{
					Resources: &v1.Resources{
						Reservations: &v1.Resource{
							Cpus: -1,
						},
					},
				},
			},
			wantErr: "cpus exceeds quota (max 16 vCPU)",
		},
		{
			name: "too much memory",
			service: &v1.Service{
				Name:  "test",
				Image: "asdf",
				Deploy: &v1.Deploy{
					Resources: &v1.Resources{
						Reservations: &v1.Resource{
							Memory: 200000,
						},
					},
				},
			},
			wantErr: "memory exceeds quota (max 65536 MiB)",
		},
		{
			name: "negative memory",
			service: &v1.Service{
				Name:  "test",
				Image: "asdf",
				Deploy: &v1.Deploy{
					Resources: &v1.Resources{
						Reservations: &v1.Resource{
							Memory: -1,
						},
					},
				},
			},
			wantErr: "memory exceeds quota (max 65536 MiB)",
		},
		{
			name: "only GPU",
			service: &v1.Service{
				Name:  "test",
				Image: "asdf",
				Deploy: &v1.Deploy{
					Resources: &v1.Resources{
						Reservations: &v1.Resource{
							Devices: []*v1.Device{
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
			service: &v1.Service{
				Name:  "test",
				Image: "asdf",
				Deploy: &v1.Deploy{
					Resources: &v1.Resources{
						Reservations: &v1.Resource{
							Devices: []*v1.Device{
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
			service: &v1.Service{
				Name:  "test",
				Image: "asdf",
				Deploy: &v1.Deploy{
					Resources: &v1.Resources{
						Reservations: &v1.Resource{
							Devices: []*v1.Device{
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
			service: &v1.Service{
				Name:  "test",
				Image: "asdf",
				Ports: []*v1.Port{{Target: 80, Mode: v1.Mode_INGRESS, Protocol: v1.Protocol_HTTP}},
				Healthcheck: &v1.HealthCheck{
					Test: []string{"CMD", "curl", "http://localhost"},
				},
				Deploy: &v1.Deploy{
					Resources: &v1.Resources{
						Reservations: &v1.Resource{
							Cpus:   1,
							Memory: 1024,
							Devices: []*v1.Device{
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
