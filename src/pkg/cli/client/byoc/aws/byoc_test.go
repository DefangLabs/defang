package aws

import (
	"bufio"
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws/ecs"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws/ecs/cfn"
	"github.com/DefangLabs/defang/src/pkg/dns"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	composeTypes "github.com/compose-spec/compose-go/v2/types"
	"github.com/stretchr/testify/assert"
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

func TestAWSEnv_ConflictingAWSCredentials(t *testing.T) {
	// Mock the STS client to avoid real AWS API calls
	originalStsFromConfig := aws.NewStsFromConfig
	aws.NewStsFromConfig = func(cfg awssdk.Config) aws.StsClientAPI {
		return &aws.MockStsClientAPI{}
	}
	t.Cleanup(func() {
		aws.NewStsFromConfig = originalStsFromConfig
	})

	// Create a temporary AWS config directory with fake credentials
	tmpDir := t.TempDir()
	configContent := `[profile my-aws-profile]
region = us-west-2

[default]
region = us-west-2
`
	//nolint:gosec // These are fake test AWS credentials
	credentialsContent := `[my-aws-profile]
aws_access_key_id = AKIAIOSFODNN7EXAMPLE
aws_secret_access_key = wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY

[default]
aws_access_key_id = AKIADEFAULT7EXAMPLE
aws_secret_access_key = wJalrXUtnFEMI/KDEFANG/bPxRfiCYEXAMPLEKEY
`
	configPath := filepath.Join(tmpDir, "config")
	credentialsPath := filepath.Join(tmpDir, "credentials")
	if err := os.WriteFile(configPath, []byte(configContent), 0600); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}
	if err := os.WriteFile(credentialsPath, []byte(credentialsContent), 0600); err != nil {
		t.Fatalf("failed to write credentials file: %v", err)
	}

	tests := []struct {
		name           string
		envVars        map[string]string
		configFiles    bool
		expectWarning  bool
		expectedError  bool
		warningContain string
	}{
		// Without config files
		{
			name:          "no AWS env vars, no config files",
			envVars:       map[string]string{},
			configFiles:   false,
			expectedError: true,
			expectWarning: false,
		},
		{
			name: "only AWS_PROFILE, no config files",
			envVars: map[string]string{
				"AWS_PROFILE": "my-aws-profile",
			},
			configFiles:   false,
			expectedError: true,
			expectWarning: false,
		},
		{
			name: "only AWS_ACCESS_KEY_ID, no config files",
			envVars: map[string]string{
				"AWS_ACCESS_KEY_ID": "AKIAIOSFODNN7EXAMPLE",
			},
			configFiles:   false,
			expectedError: true,
			expectWarning: false,
		},
		{
			name: "both AWS_PROFILE and AWS_ACCESS_KEY_ID, no config files",
			envVars: map[string]string{
				"AWS_PROFILE":       "my-aws-profile",
				"AWS_ACCESS_KEY_ID": "AKIADEFAULT7EXAMPLE",
			},
			configFiles:    false,
			expectedError:  true,
			expectWarning:  true,
			warningContain: "AWS_ACCESS_KEY_ID takes precedence",
		},
		// With config files
		{
			name:          "no AWS env vars, with config files",
			envVars:       map[string]string{},
			configFiles:   true,
			expectWarning: false,
		},
		{
			name: "only AWS_PROFILE, with config files",
			envVars: map[string]string{
				"AWS_PROFILE": "my-aws-profile",
			},
			configFiles:   true,
			expectWarning: false,
		},
		{
			name: "only AWS_ACCESS_KEY_ID, with config files",
			envVars: map[string]string{
				"AWS_ACCESS_KEY_ID": "AKIAIOSFODNN7EXAMPLE",
			},
			configFiles:   true,
			expectWarning: false,
		},
		{
			name: "both AWS_PROFILE and AWS_ACCESS_KEY_ID, with config files",
			envVars: map[string]string{
				"AWS_PROFILE":       "my-aws-profile",
				"AWS_ACCESS_KEY_ID": "AKIADEFAULT7EXAMPLE",
			},
			configFiles:    true,
			expectWarning:  true,
			warningContain: "AWS_ACCESS_KEY_ID takes precedence",
		},
		{
			name: "AWS_PROFILE with both AWS keys, with config files",
			envVars: map[string]string{
				"AWS_PROFILE":           "my-aws-profile",
				"AWS_ACCESS_KEY_ID":     "AKIADEFAULT7EXAMPLE",
				"AWS_SECRET_ACCESS_KEY": "wJalrXUtnFEMI/KDEFANG/bPxRfiCYEXAMPLEKEY",
			},
			configFiles:    true,
			expectWarning:  true,
			warningContain: "AWS_ACCESS_KEY_ID takes precedence",
		},
		// Edge cases
		{
			name: "empty AWS_PROFILE with AWS_ACCESS_KEY_ID",
			envVars: map[string]string{
				"AWS_PROFILE":       "",
				"AWS_ACCESS_KEY_ID": "AKIATEST",
			},
			configFiles:   true,
			expectWarning: false,
		},
		{
			name: "AWS_PROFILE with empty AWS_ACCESS_KEY_ID",
			envVars: map[string]string{
				"AWS_PROFILE":       "my-aws-profile",
				"AWS_ACCESS_KEY_ID": "",
			},
			configFiles:   true,
			expectWarning: false,
		},
		{
			name: "AWS_PROFILE with only AWS_SECRET_ACCESS_KEY",
			envVars: map[string]string{
				"AWS_PROFILE":           "my-aws-profile",
				"AWS_SECRET_ACCESS_KEY": "somesecret",
			},
			configFiles:   true,
			expectWarning: false,
		},
		{
			name: "AWS_DEFAULT_PROFILE with AWS_ACCESS_KEY_ID",
			envVars: map[string]string{
				"AWS_DEFAULT_PROFILE": "my-aws-profile",
				"AWS_ACCESS_KEY_ID":   "AKIATEST",
			},
			configFiles:   true,
			expectWarning: false,
		},
		{
			name: "whitespace AWS_PROFILE with AWS_ACCESS_KEY_ID",
			envVars: map[string]string{
				"AWS_PROFILE":       "   ",
				"AWS_ACCESS_KEY_ID": "AKIATEST",
			},
			configFiles:    true,
			expectedError:  true,
			expectWarning:  true,
			warningContain: "AWS_ACCESS_KEY_ID takes precedence",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, _ := term.SetupTestTerm(t)

			if tt.configFiles {
				// Point AWS SDK to our fake config files
				t.Setenv("AWS_CONFIG_FILE", configPath)
				t.Setenv("AWS_SHARED_CREDENTIALS_FILE", credentialsPath)
			}

			// Set test env vars
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}

			ctx := t.Context()

			// Create ByocAws instance - warning is printed here in NewByocProvider
			b := NewByocProvider(ctx, "tenant1", "exampleStack")
			_, err := b.AccountInfo(ctx)

			if tt.expectedError {
				assert.Error(t, err, "expected AccountInfo() to fail")
			} else {
				assert.NoError(t, err, "AccountInfo() should succeed")
			}

			// Check if warning was printed to stdout
			output := stdout.String()

			if tt.expectWarning {
				assert.Contains(t, output, tt.warningContain, "expected warning in output")
			} else {
				t.Errorf("unexpected warning output: %s", output)
			}
		})
	}
}
