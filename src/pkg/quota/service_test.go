package quota

import (
	"testing"

	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		service *defangv1.Service
		wantErr string
	}{
		{
			name:    "empty service",
			service: &defangv1.Service{},
			wantErr: "service name is required",
		},
		{
			name:    "no image, no build",
			service: &defangv1.Service{Name: "test"},
			wantErr: "missing image or build",
		},
		{
			name:    "empty build",
			service: &defangv1.Service{Name: "test", Build: &defangv1.Build{}},
			wantErr: "build.context is required",
		},
		{
			name:    "shm size exceeds quota",
			service: &defangv1.Service{Name: "test", Build: &defangv1.Build{Context: ".", ShmSize: 30721}},
			wantErr: "build.shm_size exceeds quota (max 30720 MiB)",
		},
		{
			name:    "port 0 out of range",
			service: &defangv1.Service{Name: "test", Image: "asdf", Ports: []*defangv1.Port{{Target: 0}}},
			wantErr: "port 0 is out of range",
		},
		{
			name:    "port out of range",
			service: &defangv1.Service{Name: "test", Image: "asdf", Ports: []*defangv1.Port{{Target: 33333}}},
			wantErr: "port 33333 is out of range",
		},
		{
			name:    "ingress with UDP",
			service: &defangv1.Service{Name: "test", Image: "asdf", Ports: []*defangv1.Port{{Target: 53, Mode: defangv1.Mode_INGRESS, Protocol: defangv1.Protocol_UDP}}},
			wantErr: "mode:INGRESS is not supported by protocol:UDP",
		},
		{
			name:    "ingress with UDP",
			service: &defangv1.Service{Name: "test", Image: "asdf", Ports: []*defangv1.Port{{Target: 80, Mode: defangv1.Mode_INGRESS, Protocol: defangv1.Protocol_TCP}}},
			wantErr: "mode:INGRESS is not supported by protocol:TCP",
		},
		{
			name: "invalid healthcheck interval",
			service: &defangv1.Service{
				Name:  "test",
				Image: "asdf",
				Healthcheck: &defangv1.HealthCheck{
					Test:     []string{"CMD-SHELL", "echo 1"},
					Interval: 1,
					Timeout:  2,
				},
			},
			wantErr: "invalid healthcheck: timeout must be less than the interval",
		},
		{
			name: "invalid CMD healthcheck",
			service: &defangv1.Service{
				Name:  "test",
				Image: "asdf",
				Ports: []*defangv1.Port{{Target: 80, Mode: defangv1.Mode_INGRESS, Protocol: defangv1.Protocol_HTTP}},
				Healthcheck: &defangv1.HealthCheck{
					Test: []string{"CMD", "echo 1"},
				},
			},
			wantErr: "invalid healthcheck: ingress ports require a healthcheck with HTTP URL",
		},
		{
			name: "CMD without curl or wget",
			service: &defangv1.Service{
				Name:  "test",
				Image: "asdf",
				Ports: []*defangv1.Port{{Target: 80, Mode: defangv1.Mode_INGRESS, Protocol: defangv1.Protocol_HTTP}},
				Healthcheck: &defangv1.HealthCheck{
					Test: []string{"CMD", "echo", "1"},
				},
			},
			wantErr: "invalid healthcheck: ingress ports require a healthcheck with HTTP URL",
		},
		{
			name: "CMD without HTTP URL",
			service: &defangv1.Service{
				Name:  "test",
				Image: "asdf",
				Ports: []*defangv1.Port{{Target: 80, Mode: defangv1.Mode_INGRESS, Protocol: defangv1.Protocol_HTTP}},
				Healthcheck: &defangv1.HealthCheck{
					Test: []string{"CMD", "curl", "1"},
				},
			},
			wantErr: "invalid healthcheck: ingress ports require a healthcheck with HTTP URL",
		},
		{
			name: "NONE with arguments",
			service: &defangv1.Service{
				Name:  "test",
				Image: "asdf",
				Healthcheck: &defangv1.HealthCheck{
					Test: []string{"NONE", "echo", "1"},
				},
			},
			// wantErr: "invalid NONE healthcheck; expected no arguments",
		},
		{
			name: "CMD-SHELL with ingress",
			service: &defangv1.Service{
				Name:  "test",
				Image: "asdf",
				Ports: []*defangv1.Port{{Target: 80, Mode: defangv1.Mode_INGRESS, Protocol: defangv1.Protocol_HTTP}},
				Healthcheck: &defangv1.HealthCheck{
					Test: []string{"CMD-SHELL", "echo 1"},
				},
			},
			wantErr: "invalid healthcheck: ingress ports require a healthcheck with HTTP URL",
		},
		{
			name: "NONE with ingress",
			service: &defangv1.Service{
				Name:  "test",
				Image: "asdf",
				Ports: []*defangv1.Port{{Target: 80, Mode: defangv1.Mode_INGRESS, Protocol: defangv1.Protocol_HTTP}},
				Healthcheck: &defangv1.HealthCheck{
					Test: []string{"NONE"},
				},
			},
			wantErr: "invalid healthcheck: ingress ports require a CMD or CMD-SHELL healthcheck",
		},
		{
			name: "unsupported healthcheck test",
			service: &defangv1.Service{
				Name:  "test",
				Image: "asdf",
				Healthcheck: &defangv1.HealthCheck{
					Test: []string{"BLAH"},
				},
			},
			wantErr: "unsupported healthcheck: [BLAH]",
		},
		{
			name: "too many replicas",
			service: &defangv1.Service{
				Name:  "test",
				Image: "asdf",
				Deploy: &defangv1.Deploy{
					Replicas: 100,
				},
			},
			wantErr: "replicas exceeds quota (max 16)",
		},
		{
			name: "too many CPUs",
			service: &defangv1.Service{
				Name:  "test",
				Image: "asdf",
				Deploy: &defangv1.Deploy{
					Resources: &defangv1.Resources{
						Reservations: &defangv1.Resource{
							Cpus: 100,
						},
					},
				},
			},
			wantErr: "cpus exceeds quota (max 16 vCPU)",
		},
		{
			name: "negative cpus",
			service: &defangv1.Service{
				Name:  "test",
				Image: "asdf",
				Deploy: &defangv1.Deploy{
					Resources: &defangv1.Resources{
						Reservations: &defangv1.Resource{
							Cpus: -1,
						},
					},
				},
			},
			wantErr: "cpus exceeds quota (max 16 vCPU)",
		},
		{
			name: "too much memory",
			service: &defangv1.Service{
				Name:  "test",
				Image: "asdf",
				Deploy: &defangv1.Deploy{
					Resources: &defangv1.Resources{
						Reservations: &defangv1.Resource{
							Memory: 200000,
						},
					},
				},
			},
			wantErr: "memory exceeds quota (max 65536 MiB)",
		},
		{
			name: "negative memory",
			service: &defangv1.Service{
				Name:  "test",
				Image: "asdf",
				Deploy: &defangv1.Deploy{
					Resources: &defangv1.Resources{
						Reservations: &defangv1.Resource{
							Memory: -1,
						},
					},
				},
			},
			wantErr: "memory exceeds quota (max 65536 MiB)",
		},
		{
			name: "only GPU",
			service: &defangv1.Service{
				Name:  "test",
				Image: "asdf",
				Deploy: &defangv1.Deploy{
					Resources: &defangv1.Resources{
						Reservations: &defangv1.Resource{
							Devices: []*defangv1.Device{
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
			service: &defangv1.Service{
				Name:  "test",
				Image: "asdf",
				Deploy: &defangv1.Deploy{
					Resources: &defangv1.Resources{
						Reservations: &defangv1.Resource{
							Devices: []*defangv1.Device{
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
			service: &defangv1.Service{
				Name:  "test",
				Image: "asdf",
				Deploy: &defangv1.Deploy{
					Resources: &defangv1.Resources{
						Reservations: &defangv1.Resource{
							Devices: []*defangv1.Device{
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
			service: &defangv1.Service{
				Name:  "test",
				Image: "asdf",
				Ports: []*defangv1.Port{{Target: 80, Mode: defangv1.Mode_INGRESS, Protocol: defangv1.Protocol_HTTP}},
				Healthcheck: &defangv1.HealthCheck{
					Test: []string{"CMD", "curl", "http://localhost"},
				},
				Deploy: &defangv1.Deploy{
					Resources: &defangv1.Resources{
						Reservations: &defangv1.Resource{
							Cpus:   1,
							Memory: 1024,
							Devices: []*defangv1.Device{
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
			if err := byoc.Validate(tt.service); err != nil && err.Error() != tt.wantErr {
				t.Errorf("Byoc.Validate() = %q, want %q", err, tt.wantErr)
			} else if err == nil && tt.wantErr != "" {
				t.Errorf("Byoc.Validate() = nil, want %q", tt.wantErr)
			}
		})
	}
}
