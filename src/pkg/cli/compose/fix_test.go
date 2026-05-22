package compose

import (
	"reflect"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/modes"
	composeTypes "github.com/compose-spec/compose-go/v2/types"
)

func TestFixProject(t *testing.T) {
	tests := []struct {
		name    string
		project *Project
		want    []FixResult
		check   func(*testing.T, *Project)
	}{
		{
			name: "web service defaults",
			project: &Project{Services: Services{
				"web": {
					Name:  "web",
					Image: "nginx",
					Ports: []composeTypes.ServicePortConfig{{Target: 8080}},
				},
			}},
			want: []FixResult{
				{Service: "web", Field: "mode", Action: "added", After: Mode_INGRESS, Reason: "port 8080"},
				{Service: "web", Field: "deploy.resources.reservations.memory", Action: "added", After: "512M", Reason: "missing memory reservation"},
				{Service: "web", Field: "restart", Action: "added", After: defaultRestartPolicy, Reason: "missing restart policy"},
				{Service: "web", Field: "healthcheck", Action: "added", After: "CMD curl -f http://localhost:8080/", Reason: "ingress port 8080"},
			},
			check: func(t *testing.T, project *Project) {
				service := project.Services["web"]
				if service.Ports[0].Mode != Mode_INGRESS {
					t.Fatalf("port mode = %q, want %q", service.Ports[0].Mode, Mode_INGRESS)
				}
				if service.HealthCheck == nil {
					t.Fatal("healthcheck was not added")
				}
				if service.Deploy.Resources.Reservations.MemoryBytes != 512*MiB {
					t.Fatalf("memory = %d, want %d", service.Deploy.Resources.Reservations.MemoryBytes, 512*MiB)
				}
			},
		},
		{
			name: "managed postgres defaults to host",
			project: &Project{Services: Services{
				"db": {
					Name:  "db",
					Image: "postgres:16",
					Ports: []composeTypes.ServicePortConfig{{Target: 5432}},
				},
			}},
			want: []FixResult{
				{Service: "db", Field: "mode", Action: "added", After: Mode_HOST, Reason: "port 5432 (database image)"},
				{Service: "db", Field: "x-defang-postgres", Action: "added", After: "true", Reason: "postgres image detected"},
				{Service: "db", Field: "deploy.resources.reservations.memory", Action: "added", After: "512M", Reason: "missing memory reservation"},
				{Service: "db", Field: "restart", Action: "added", After: defaultRestartPolicy, Reason: "missing restart policy"},
			},
			check: func(t *testing.T, project *Project) {
				service := project.Services["db"]
				if service.Ports[0].Mode != Mode_HOST {
					t.Fatalf("port mode = %q, want %q", service.Ports[0].Mode, Mode_HOST)
				}
				if service.Extensions["x-defang-postgres"] != true {
					t.Fatal("x-defang-postgres was not added")
				}
			},
		},
		{
			name: "limits copied to reservations",
			project: &Project{Services: Services{
				"api": {
					Name:    "api",
					Image:   "api",
					Restart: defaultRestartPolicy,
					Deploy: &composeTypes.DeployConfig{Resources: composeTypes.Resources{
						Limits: &composeTypes.Resource{MemoryBytes: 1024 * MiB},
					}},
				},
			}},
			want: []FixResult{
				{Service: "api", Field: "deploy.resources.reservations", Action: "added", After: "deploy.resources.limits", Reason: "limits used as reservations"},
			},
			check: func(t *testing.T, project *Project) {
				service := project.Services["api"]
				if service.Deploy.Resources.Reservations == nil {
					t.Fatal("reservations were not added")
				}
				if service.Deploy.Resources.Reservations.MemoryBytes != 1024*MiB {
					t.Fatalf("memory = %d, want %d", service.Deploy.Resources.Reservations.MemoryBytes, 1024*MiB)
				}
			},
		},
		{
			name: "unsupported directives removed",
			project: &Project{Services: Services{
				"worker": {
					Name:              "worker",
					Image:             "worker",
					Restart:           defaultRestartPolicy,
					DNS:               composeTypes.StringList{"1.1.1.1"},
					DNSSearch:         composeTypes.StringList{"example.com"},
					Devices:           []composeTypes.DeviceMapping{{Source: "/dev/null", Target: "/dev/null"}},
					DeviceCgroupRules: []string{"c 1:3 mr"},
					GroupAdd:          []string{"audio"},
				},
			}},
			want: []FixResult{
				{Service: "worker", Field: "deploy.resources.reservations.memory", Action: "added", After: "512M", Reason: "missing memory reservation"},
				{Service: "worker", Field: "dns", Action: "removed", Before: "present", Reason: "unsupported directive"},
				{Service: "worker", Field: "dns_search", Action: "removed", Before: "present", Reason: "unsupported directive"},
				{Service: "worker", Field: "devices", Action: "removed", Before: "present", Reason: "unsupported directive"},
				{Service: "worker", Field: "device_cgroup_rules", Action: "removed", Before: "present", Reason: "unsupported directive"},
				{Service: "worker", Field: "group_add", Action: "removed", Before: "present", Reason: "unsupported directive"},
			},
			check: func(t *testing.T, project *Project) {
				service := project.Services["worker"]
				if len(service.DNS) != 0 || len(service.DNSSearch) != 0 || len(service.Devices) != 0 || len(service.DeviceCgroupRules) != 0 || len(service.GroupAdd) != 0 {
					t.Fatal("unsupported directives were not removed")
				}
			},
		},
		{
			name: "hostname and deploy restart policy",
			project: &Project{Services: Services{
				"api": {
					Name:     "api",
					Image:    "api",
					Hostname: "api.example.com",
					Deploy: &composeTypes.DeployConfig{
						RestartPolicy: &composeTypes.RestartPolicy{Condition: "any"},
					},
				},
			}},
			want: []FixResult{
				{Service: "api", Field: "deploy.resources.reservations.memory", Action: "added", After: "512M", Reason: "missing memory reservation"},
				{Service: "api", Field: "restart", Action: "added", After: "always", Reason: "deploy.restart_policy is unsupported"},
				{Service: "api", Field: "domainname", Action: "changed", Before: "api.example.com", After: "api.example.com", Reason: "hostname is unsupported"},
			},
			check: func(t *testing.T, project *Project) {
				service := project.Services["api"]
				if service.Hostname != "" || service.DomainName != "api.example.com" {
					t.Fatalf("hostname/domainname = %q/%q", service.Hostname, service.DomainName)
				}
				if service.Deploy.RestartPolicy != nil {
					t.Fatal("deploy.restart_policy was not removed")
				}
			},
		},
		{
			name: "static files memory default",
			project: &Project{Services: Services{
				"cdn": {
					Name:       "cdn",
					Image:      "nginx",
					Restart:    defaultRestartPolicy,
					Extensions: composeTypes.Extensions{"x-defang-static-files": "./public"},
				},
			}},
			want: []FixResult{
				{Service: "cdn", Field: "deploy.resources.reservations.memory", Action: "added", After: "256M", Reason: "missing memory reservation"},
			},
			check: func(t *testing.T, project *Project) {
				service := project.Services["cdn"]
				if service.Deploy.Resources.Reservations.MemoryBytes != 256*MiB {
					t.Fatalf("memory = %d, want %d", service.Deploy.Resources.Reservations.MemoryBytes, 256*MiB)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FixProject(tt.project)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("FixProject() = %#v, want %#v", got, tt.want)
			}
			tt.check(t, tt.project)
		})
	}
}

func TestFixProjectOutputValidates(t *testing.T) {
	loader := NewLoader(WithPath("../../../testdata/compose-fix/compose.yaml"))
	project, err := loader.LoadProject(t.Context())
	if err != nil {
		t.Fatalf("LoadProject() failed: %v", err)
	}

	if fixes := FixProject(project); len(fixes) == 0 {
		t.Fatal("expected fixes")
	}
	if err := ValidateProject(project, modes.ModeUnspecified); err != nil {
		t.Fatalf("ValidateProject() after FixProject() failed: %v", err)
	}
}
