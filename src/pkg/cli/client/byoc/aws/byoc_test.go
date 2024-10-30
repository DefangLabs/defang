package aws

import (
	"bufio"
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"io"
	"os"
	"path"
	"strings"
	"sync"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws/ecs"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	composeTypes "github.com/compose-spec/compose-go/v2/types"
)

func TestDomainMultipleProjectSupport(t *testing.T) {
	port80 := &composeTypes.ServicePortConfig{Mode: compose.Mode_INGRESS, Target: 80}
	port8080 := &composeTypes.ServicePortConfig{Mode: compose.Mode_INGRESS, Target: 8080}
	hostModePort := &composeTypes.ServicePortConfig{Mode: compose.Mode_HOST, Target: 80}
	tests := []struct {
		Region      string
		ProjectName string
		TenantID    types.TenantID
		Fqn         string
		Port        *composeTypes.ServicePortConfig
		EndPoint    string
		PublicFqdn  string
		PrivateFqdn string
	}{
		{"us-west-2", "tenant1", "tenant1", "web", port80, "web--80.example.com", "web.example.com", "web.tenant1.internal"},
		{"us-west-2", "tenant1", "tenant1", "web", hostModePort, "web.tenant1.internal:80", "web.example.com", "web.tenant1.internal"},
		{"us-west-2", "project1", "tenant1", "web", port80, "web--80.project1.example.com", "web.project1.example.com", "web.project1.internal"},
		{"us-west-2", "Project1", "tenant1", "web", port80, "web--80.project1.example.com", "web.project1.example.com", "web.project1.internal"},
		{"us-west-2", "project1", "tenant1", "web", hostModePort, "web.project1.internal:80", "web.project1.example.com", "web.project1.internal"},
		{"us-west-2", "project1", "tenant1", "api", port8080, "api--8080.project1.example.com", "api.project1.example.com", "api.project1.internal"},
		{"us-west-2", "tenant1", "tenant1", "web", port80, "web--80.example.com", "web.example.com", "web.tenant1.internal"},
		{"us-west-2", "tenant1", "tenant1", "web", hostModePort, "web.tenant1.internal:80", "web.example.com", "web.tenant1.internal"},
		{"us-west-2", "Project1", "tenant1", "web", port80, "web--80.project1.example.com", "web.project1.example.com", "web.project1.internal"},
		{"us-west-2", "Tenant2", "tenant1", "web", port80, "web--80.tenant2.example.com", "web.tenant2.example.com", "web.tenant2.internal"},
		{"us-west-2", "tenant1", "tenAnt1", "web", port80, "web--80.example.com", "web.example.com", "web.tenant1.internal"},
		{"", "tenant1", "tenant1", "web", port80, "web--80.example.com", "web.example.com", "web.tenant1.internal"},
	}

	origRegion := os.Getenv("AWS_REGION")
	defer func() { os.Setenv("AWS_REGION", origRegion) }()
	for _, tt := range tests {
		t.Run(tt.ProjectName+","+string(tt.TenantID), func(t *testing.T) {
			os.Setenv("AWS_REGION", tt.Region)
			grpcClient := &client.GrpcClient{Loader: FakeLoader{ProjectName: tt.ProjectName}}
			b, err := NewByocClient(context.Background(), *grpcClient, tt.TenantID)
			if err != nil {
				if tt.Region == "" {
					return
				}
				t.Fatalf("NewByocClient() failed: %v", err)
			}
			if _, err := b.LoadProject(context.Background()); err != nil {
				t.Fatalf("LoadProject() failed: %v", err)
			}
			b.ProjectDomain = b.getProjectDomain("example.com")

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

func (f FakeLoader) LoadProject(ctx context.Context) (*composeTypes.Project, error) {
	return &composeTypes.Project{Name: f.ProjectName}, nil
}

func (f FakeLoader) LoadProjectName(ctx context.Context) (string, error) {
	return f.ProjectName, nil
}

//go:embed test_ecs_events/*.json
var testDir embed.FS

//go:embed test_ecs_events/*.events
var expectedDir embed.FS

func TestSubscribe(t *testing.T) {
	t.Skip("Pending test")
	tests, err := testDir.ReadDir("test_ecs_events")
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

			resp, err := byoc.Subscribe(context.Background(), &defangv1.SubscribeRequest{
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

				filename := path.Join("test_ecs_events", name+".events")
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

			data, err := testDir.ReadFile(path.Join("test_ecs_events", tt.Name()))
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

func TestNewByocClient(t *testing.T) {
	tests := []struct {
		Region        string
		Name          string
		ExpectedError bool
	}{
		{"us-west-2", "AWS Region set", false},
		{"", "AWS Region not set", true}}

	origRegion := os.Getenv("AWS_REGION")
	defer func() { os.Setenv("AWS_REGION", origRegion) }()
	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			os.Setenv("AWS_REGION", tt.Region)
			grpcClient := &client.GrpcClient{Loader: FakeLoader{ProjectName: "project1"}}
			_, err := NewByocClient(context.Background(), *grpcClient, "tenant1")
			errorOccured := (err != nil)
			if errorOccured != tt.ExpectedError {
				t.Fatalf("Failed: expected error to show %t, but got %t", tt.ExpectedError, errorOccured)
			}
		})
	}
}
