package aws

import (
	"bufio"
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws/ecs"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws/ecs/cfn"
	"github.com/DefangLabs/defang/src/pkg/dns"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	composeTypes "github.com/compose-spec/compose-go/v2/types"
)

func TestDomainMultipleProjectSupport(t *testing.T) {
	port80 := &composeTypes.ServicePortConfig{Mode: "ingress", Target: 80}
	port8080 := &composeTypes.ServicePortConfig{Mode: "ingress", Target: 8080}
	hostModePort := &composeTypes.ServicePortConfig{Mode: "host", Target: 80}
	tests := []struct {
		ProjectName string
		TenantLabel types.TenantLabel
		Fqn         string
		Port        *composeTypes.ServicePortConfig
		EndPoint    string
		PublicFqdn  string
		PrivateFqdn string
	}{
		{"tenant1", "tenant1", "web", port80, "web--80.example.com", "web.example.com", "web.tenant1.internal"},
		{"tenant1", "tenant1", "web", hostModePort, "web.tenant1.internal:80", "web.example.com", "web.tenant1.internal"},
		{"project1", "tenant1", "web", port80, "web--80.project1.example.com", "web.project1.example.com", "web.project1.internal"},
		{"Project1", "tenant1", "web", port80, "web--80.project1.example.com", "web.project1.example.com", "web.project1.internal"},
		{"project1", "tenant1", "web", hostModePort, "web.project1.internal:80", "web.project1.example.com", "web.project1.internal"},
		{"project1", "tenant1", "api", port8080, "api--8080.project1.example.com", "api.project1.example.com", "api.project1.internal"},
		{"tenant1", "tenant1", "web", port80, "web--80.example.com", "web.example.com", "web.tenant1.internal"},
		{"tenant1", "tenant1", "web", hostModePort, "web.tenant1.internal:80", "web.example.com", "web.tenant1.internal"},
		{"Project1", "tenant1", "web", port80, "web--80.project1.example.com", "web.project1.example.com", "web.project1.internal"},
		{"Tenant2", "tenant1", "web", port80, "web--80.tenant2.example.com", "web.tenant2.example.com", "web.tenant2.internal"},
		{"tenant1", "tenAnt1", "web", port80, "web--80.example.com", "web.example.com", "web.tenant1.internal"},
		{"tenant1", "tenant1", "web", port80, "web--80.example.com", "web.example.com", "web.tenant1.internal"},
	}

	for _, tt := range tests {
		t.Run(tt.ProjectName+","+string(tt.TenantLabel), func(t *testing.T) {
			//like calling NewByocProvider(), but without needing real AccountInfo data
			b := &ByocAws{
				driver: cfn.New(byoc.CdTaskPrefix, aws.Region("")), // default region
			}
			b.ByocBaseClient = byoc.NewByocBaseClient(tt.TenantLabel, b, "")

			delegateDomain := "example.com"
			projectLabel := dns.SafeLabel(tt.ProjectName)
			tenantLabel := dns.SafeLabel(string(tt.TenantLabel))
			if projectLabel != tenantLabel { // avoid stuttering
				delegateDomain = projectLabel + "." + delegateDomain
			}

			endpoint := b.GetEndpoint(tt.Fqn, tt.ProjectName, delegateDomain, tt.Port)
			if endpoint != tt.EndPoint {
				t.Errorf("expected endpoint %q, got %q", tt.EndPoint, endpoint)
			}

			publicFqdn := b.GetPublicFqdn(tt.ProjectName, delegateDomain, tt.Fqn)
			if publicFqdn != tt.PublicFqdn {
				t.Errorf("expected public fqdn %q, got %q", tt.PublicFqdn, publicFqdn)
			}

			privateFqdn := b.GetPrivateFqdn(tt.ProjectName, tt.Fqn)
			if privateFqdn != tt.PrivateFqdn {
				t.Errorf("expected private fqdn %q, got %q", tt.PrivateFqdn, privateFqdn)
			}
		})
	}
}

type FakeLoader struct {
	ProjectName string
}

func (f FakeLoader) LoadProject(ctx context.Context) (*composeTypes.Project, error) {
	return &composeTypes.Project{Name: f.ProjectName}, nil
}

func (f FakeLoader) LoadProjectName(ctx context.Context) (string, error) {
	return f.ProjectName, nil
}

//go:embed testdata/*.json
var testDir embed.FS

//go:embed testdata/*.events
var expectedDir embed.FS

func TestSubscribe(t *testing.T) {
	t.Skip("Pending test")
	tests, err := testDir.ReadDir("testdata")
	if err != nil {
		t.Fatalf("failed to load ecs events test files: %v", err)
	}
	for _, tt := range tests {
		t.Run(tt.Name(), func(t *testing.T) {
			start := strings.LastIndex(tt.Name(), "-")
			end := strings.LastIndex(tt.Name(), ".")
			if start == -1 || end == -1 {
				t.Fatalf("cannot find etag from invalid test file name: %s", tt.Name())
			}
			name := tt.Name()[:start]
			etag := tt.Name()[start+1 : end]

			byoc := &ByocAws{}

			resp, err := byoc.Subscribe(t.Context(), &defangv1.SubscribeRequest{
				Etag:     etag,
				Services: []string{"api", "web"},
			})
			if err != nil {
				t.Fatalf("Subscribe() failed: %v", err)
			}

			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				defer wg.Done()

				filename := filepath.Join("testdata", name+".events")
				ef, _ := expectedDir.ReadFile(filename)
				dec := json.NewDecoder(bytes.NewReader(ef))

				for {
					if !resp.Receive() {
						if resp.Err() != nil {
							t.Errorf("Receive() failed: %v", resp.Err())
						}
						break
					}
					msg := resp.Msg()
					var expected defangv1.SubscribeResponse
					if err := dec.Decode(&expected); err == io.EOF {
						t.Errorf("unexpected message: %v", msg)
					} else if err != nil {
						t.Errorf("error unmarshaling expected ECS event: %v", err)
					} else if msg.Name != expected.Name || msg.Status != expected.Status || msg.State != expected.State {
						t.Errorf("expected message-, got+\n-%v\n+%v", &expected, msg)
					}
				}
			}()

			data, err := testDir.ReadFile(filepath.Join("testdata", tt.Name()))
			if err != nil {
				t.Fatalf("failed to read test file: %v", err)
			}
			lines := bufio.NewScanner(bytes.NewReader(data))
			for lines.Scan() {
				ecsEvt, err := ecs.ParseECSEvent([]byte(lines.Text()))
				if err != nil {
					t.Fatalf("error parsing ECS event: %v", err)
				}

				byoc.HandleECSEvent(ecsEvt)
			}
			resp.Close()

			wg.Wait()
		})
	}
}
