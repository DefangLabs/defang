package byoc

import (
	"github.com/defang-io/defang/src/pkg/cli/client/byoc/clouds"
	"testing"

	"github.com/defang-io/defang/src/pkg/types"
	v1 "github.com/defang-io/defang/src/protos/io/defang/v1"
)

func TestDomainMultipleProjectSupport(t *testing.T) {
	port80 := &v1.Port{Mode: v1.Mode_INGRESS, Target: 80}
	port8080 := &v1.Port{Mode: v1.Mode_INGRESS, Target: 8080}
	hostModePort := &v1.Port{Mode: v1.Mode_HOST, Target: 80}
	tests := []struct {
		ProjectName string
		TenantID    types.TenantID
		Fqn         string
		Port        *v1.Port
		EndPoint    string
		PublicFqdn  string
		PrivateFqdn string
	}{
		{"", "tenant1", "web", port80, "web--80.example.com", "web.example.com", "web.internal"},
		{"", "tenant1", "web", hostModePort, "web.internal:80", "web.example.com", "web.internal"},
		{"project1", "tenant1", "web", port80, "web--80.project1.example.com", "web.project1.example.com", "web.project1.internal"},
		{"Project1", "tenant1", "web", port80, "web--80.project1.example.com", "web.project1.example.com", "web.project1.internal"},
		{"project1", "tenant1", "web", hostModePort, "web.project1.internal:80", "web.project1.example.com", "web.project1.internal"},
		{"project1", "tenant1", "api", port8080, "api--8080.project1.example.com", "api.project1.example.com", "api.project1.internal"},
		{"tenant1", "tenant1", "web", port80, "web--80.example.com", "web.example.com", "web.internal"},
		{"tenant1", "tenant1", "web", hostModePort, "web.internal:80", "web.example.com", "web.internal"},
		{"Project1", "tenant1", "web", port80, "web--80.project1.example.com", "web.project1.example.com", "web.project1.internal"},
		{"Tenant2", "tenant1", "web", port80, "web--80.tenant2.example.com", "web.tenant2.example.com", "web.tenant2.internal"},
		{"tenant1", "tenAnt1", "web", port80, "web--80.example.com", "web.example.com", "web.internal"},
	}

	for _, tt := range tests {
		t.Run(tt.ProjectName+","+string(tt.TenantID), func(t *testing.T) {
			b := clouds.NewByocAWS(tt.TenantID, tt.ProjectName, nil)
			b.CustomDomain = "example.com"

			endpoint := b.GetEndpoint(tt.Fqn, tt.Port)
			if endpoint != tt.EndPoint {
				t.Errorf("expected endpoint %q, got %q", tt.EndPoint, endpoint)
			}

			publicFqdn := b.GetPublicFqdn(tt.Fqn)
			if publicFqdn != tt.PublicFqdn {
				t.Errorf("expected public fqdn %q, got %q", tt.PublicFqdn, publicFqdn)
			}

			privateFqdn := b.GetPrivateFqdn(tt.Fqn)
			if privateFqdn != tt.PrivateFqdn {
				t.Errorf("expected private fqdn %q, got %q", tt.PrivateFqdn, privateFqdn)
			}
		})
	}
}
