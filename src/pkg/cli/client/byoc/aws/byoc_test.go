package aws

import (
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	compose "github.com/compose-spec/compose-go/v2/types"
)

func TestDomainMultipleProjectSupport(t *testing.T) {
	port80 := &defangv1.Port{Mode: defangv1.Mode_INGRESS, Target: 80}
	port8080 := &defangv1.Port{Mode: defangv1.Mode_INGRESS, Target: 8080}
	hostModePort := &defangv1.Port{Mode: defangv1.Mode_HOST, Target: 80}
	tests := []struct {
		ProjectName string
		TenantID    types.TenantID
		Fqn         string
		Port        *defangv1.Port
		EndPoint    string
		PublicFqdn  string
		PrivateFqdn string
	}{
		{"", "tenant1", "web", port80, "web--80.example.com", "web.example.com", "web.tenant1.internal"},
		{"", "tenant1", "web", hostModePort, "web.tenant1.internal:80", "web.example.com", "web.tenant1.internal"},
		{"project1", "tenant1", "web", port80, "web--80.project1.example.com", "web.project1.example.com", "web.project1.internal"},
		{"Project1", "tenant1", "web", port80, "web--80.project1.example.com", "web.project1.example.com", "web.project1.internal"},
		{"project1", "tenant1", "web", hostModePort, "web.project1.internal:80", "web.project1.example.com", "web.project1.internal"},
		{"project1", "tenant1", "api", port8080, "api--8080.project1.example.com", "api.project1.example.com", "api.project1.internal"},
		{"tenant1", "tenant1", "web", port80, "web--80.example.com", "web.example.com", "web.tenant1.internal"},
		{"tenant1", "tenant1", "web", hostModePort, "web.tenant1.internal:80", "web.example.com", "web.tenant1.internal"},
		{"Project1", "tenant1", "web", port80, "web--80.project1.example.com", "web.project1.example.com", "web.project1.internal"},
		{"Tenant2", "tenant1", "web", port80, "web--80.tenant2.example.com", "web.tenant2.example.com", "web.tenant2.internal"},
		{"tenant1", "tenAnt1", "web", port80, "web--80.example.com", "web.example.com", "web.tenant1.internal"},
	}

	for _, tt := range tests {
		t.Run(tt.ProjectName+","+string(tt.TenantID), func(t *testing.T) {
			grpcClient := &client.GrpcClient{Loader: FakeLoader{ProjectName: tt.ProjectName}}
			baseClient := byoc.NewByocBaseClient(grpcClient, tt.TenantID)
			b := NewByoc(baseClient)
			if _, err := b.LoadProject(); err != nil {
				t.Fatalf("LoadProject() failed: %v", err)
			}
			b.CustomDomain = b.getProjectDomain("example.com")

			endpoint := b.getEndpoint(tt.Fqn, tt.Port)
			if endpoint != tt.EndPoint {
				t.Errorf("expected endpoint %q, got %q", tt.EndPoint, endpoint)
			}

			publicFqdn := b.getPublicFqdn(tt.Fqn)
			if publicFqdn != tt.PublicFqdn {
				t.Errorf("expected public fqdn %q, got %q", tt.PublicFqdn, publicFqdn)
			}

			privateFqdn := b.getPrivateFqdn(tt.Fqn)
			if privateFqdn != tt.PrivateFqdn {
				t.Errorf("expected private fqdn %q, got %q", tt.PrivateFqdn, privateFqdn)
			}
		})
	}
}

type FakeLoader struct {
	ProjectName string
}

func (f FakeLoader) LoadWithDefaultProjectName(defaultName string) (*compose.Project, error) {
	name := defaultName
	if f.ProjectName != "" {
		name = f.ProjectName
	}
	return &compose.Project{Name: name}, nil
}

func (f FakeLoader) LoadWithProjectName(projectName string) (*compose.Project, error) {
	return &compose.Project{Name: projectName}, nil
}
