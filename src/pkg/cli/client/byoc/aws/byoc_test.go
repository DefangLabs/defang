package aws

import (
	"bufio"
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws/cw"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws/ecs/cfn"
	"github.com/DefangLabs/defang/src/pkg/dns"
	"github.com/DefangLabs/defang/src/pkg/logs"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	cwTypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	composeTypes "github.com/compose-spec/compose-go/v2/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
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

func (f FakeLoader) LoadProjectName(ctx context.Context) (string, bool, error) {
	return f.ProjectName, false, nil
}

//go:embed testdata/*.json
var testDir embed.FS

//go:embed testdata/*.events
var expectedDir embed.FS

func TestSubscribe(t *testing.T) {
	t.Skip("Pending test") // TODO: requires CW mock or real AWS credentials
	tests, err := testDir.ReadDir("testdata")
	if err != nil {
		t.Fatalf("failed to load ecs events test files: %v", err)
	}
	for _, tt := range tests {
		if !strings.HasSuffix(tt.Name(), ".json") {
			continue
		}
		t.Run(tt.Name(), func(t *testing.T) {
			start := strings.LastIndex(tt.Name(), "-")
			end := strings.LastIndex(tt.Name(), ".")
			if start == -1 || end == -1 {
				t.Fatalf("cannot find etag from invalid test file name: %s", tt.Name())
			}
			name := tt.Name()[:start]
			etag := tt.Name()[start+1 : end]

			data, err := testDir.ReadFile(filepath.Join("testdata", tt.Name()))
			if err != nil {
				t.Fatalf("failed to read test file: %v", err)
			}

			// Build CW log events from the ECS event JSON lines
			ecsLogGroup := "arn:aws:logs:us-west-2:123:log-group:/ecs"
			lines := bufio.NewScanner(bytes.NewReader(data))
			var cwEvents []cw.LogEvent
			var ts int64
			for lines.Scan() {
				line := lines.Text()
				cwEvents = append(cwEvents, cw.LogEvent{
					LogGroupIdentifier: &ecsLogGroup,
					LogStreamName:      awssdk.String("some-stream"),
					Message:            awssdk.String(line),
					Timestamp:          &ts,
				})
			}

			// Feed through parseSubscribeEvents
			evtIter := func(yield func([]cw.LogEvent, error) bool) {
				for _, evt := range cwEvents {
					if !yield([]cw.LogEvent{evt}, nil) {
						return
					}
				}
			}

			filename := filepath.Join("testdata", name+".events")
			ef, _ := expectedDir.ReadFile(filename)
			dec := json.NewDecoder(bytes.NewReader(ef))

			for msg, err := range parseSubscribeEvents(evtIter, etag, []string{"api", "web"}) {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
					break
				}
				var expected defangv1.SubscribeResponse
				if err := dec.Decode(&expected); err == io.EOF {
					t.Errorf("unexpected message: %v", msg)
				} else if err != nil {
					t.Errorf("error unmarshaling expected event: %v", err)
				} else if msg.Name != expected.Name || msg.Status != expected.Status || msg.State != expected.State {
					t.Errorf("expected message-, got+\n-%v\n+%v", &expected, msg)
				}
			}
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

	envVars := []string{
		"AWS_PROFILE",
		"AWS_ACCESS_KEY_ID",
		"AWS_SECRET_ACCESS_KEY",
		"AWS_CONFIG_FILE",
		"AWS_SHARED_CREDENTIALS_FILE",
	}

	for _, envVar := range envVars {
		t.Setenv(envVar, "")
	}

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
			warningContain: "Partial credentials found in env, missing: AWS_SECRET_ACCESS_KEY",
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
			warningContain: "Partial credentials found in env, missing: AWS_SECRET_ACCESS_KEY",
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
			warningContain: "access keys take precedence",
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
			warningContain: "Partial credentials found in env, missing: AWS_SECRET_ACCESS_KEY",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, _ := term.SetupTestTerm(t)

			if tt.configFiles {
				// Point AWS SDK to our fake config files
				t.Setenv("AWS_CONFIG_FILE", configPath)
				t.Setenv("AWS_SHARED_CREDENTIALS_FILE", credentialsPath)
			} else {
				// Point AWS SDK to non-existent files to prevent finding real credentials
				t.Setenv("AWS_CONFIG_FILE", filepath.Join(tmpDir, "nonexistent_config"))
				t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(tmpDir, "nonexistent_credentials"))
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
				assert.NotContains(t, output, "AWS_ACCESS_KEY_ID takes precedence", "unexpected warning in output")
			}
		})
	}
}

// mockCWClient implements cw.LogsClient for testing queryLogs and queryCdLogs.
type mockCWClient struct {
	events []cwTypes.FilteredLogEvent
}

func (m *mockCWClient) FilterLogEvents(ctx context.Context, input *cloudwatchlogs.FilterLogEventsInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.FilterLogEventsOutput, error) {
	events := m.events
	if input.Limit != nil && int(*input.Limit) < len(events) {
		events = events[:*input.Limit]
	}
	return &cloudwatchlogs.FilterLogEventsOutput{
		Events: events,
	}, nil
}

func (m *mockCWClient) StartLiveTail(ctx context.Context, input *cloudwatchlogs.StartLiveTailInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.StartLiveTailOutput, error) {
	return nil, &cwTypes.ResourceNotFoundException{
		Message: awssdk.String("mock: log group does not exist"),
	}
}

// makeMockEvents creates n FilteredLogEvents with sequential timestamps and messages.
// The log stream name follows the awslogs format: "<service>/<service>_<etag>/<taskID>"
func makeMockEvents(n int, service, etag string) []cwTypes.FilteredLogEvent {
	events := make([]cwTypes.FilteredLogEvent, n)
	for i := range events {
		ts := int64((i + 1) * 1000) // 1000, 2000, 3000, ...
		events[i] = cwTypes.FilteredLogEvent{
			Message:       awssdk.String(fmt.Sprintf("log message %d", i+1)),
			Timestamp:     &ts,
			LogStreamName: awssdk.String(fmt.Sprintf("%s/%s_%s/task%d", service, service, etag, i)),
		}
	}
	return events
}

func newTestByocAws() *ByocAws {
	b := &ByocAws{
		driver: cfn.New(byoc.CdTaskPrefix, aws.Region("us-test-2")),
	}
	b.driver.AccountID = "123456789012"
	b.driver.LogGroupARN = "arn:aws:logs:us-test-2:123456789012:log-group:defang-cd-LogGroup:*"
	b.driver.ClusterName = "test-cluster"
	b.ByocBaseClient = byoc.NewByocBaseClient("tenant1", b, "beta")
	return b
}

func collectEvents(t *testing.T, iter func(func(cw.LogEvent, error) bool)) []cw.LogEvent {
	t.Helper()
	var events []cw.LogEvent
	for evt, err := range iter {
		require.NoError(t, err)
		events = append(events, evt)
	}
	return events
}

func TestQueryLogs(t *testing.T) {
	const etag = "hg2xsgvsldqk"
	baseTime := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		req       *defangv1.TailRequest
		numEvents int // how many mock events to create
		wantCount int // expected number of events returned
		wantFirst string
		wantLast  string
	}{
		{
			name: "query, no limit, no times",
			req: &defangv1.TailRequest{
				Services: []string{"app"},
				Project:  "testproject",
				LogType:  uint32(logs.LogTypeRun),
			},
			numEvents: 5,
			wantCount: 5,
			wantFirst: "log message 1",
			wantLast:  "log message 5",
		},
		{
			name: "query, no limit, with start",
			req: &defangv1.TailRequest{
				Services: []string{"app"},
				Since:    timestamppb.New(baseTime),
				Project:  "testproject",
				LogType:  uint32(logs.LogTypeRun),
			},
			numEvents: 5,
			wantCount: 5,
			wantFirst: "log message 1",
			wantLast:  "log message 5",
		},
		{
			name: "query, no limit, with start and end",
			req: &defangv1.TailRequest{
				Services: []string{"app"},
				Since:    timestamppb.New(baseTime),
				Until:    timestamppb.New(baseTime.Add(time.Hour)),
				Project:  "testproject",
				LogType:  uint32(logs.LogTypeRun),
			},
			numEvents: 5,
			wantCount: 5,
			wantFirst: "log message 1",
			wantLast:  "log message 5",
		},
		{
			name: "query, limit 3, with start (TakeFirstN)",
			req: &defangv1.TailRequest{
				Services: []string{"app"},
				Since:    timestamppb.New(baseTime),
				Limit:    3,
				Project:  "testproject",
				LogType:  uint32(logs.LogTypeRun),
			},
			numEvents: 5,
			wantCount: 3,
			wantFirst: "log message 1",
			wantLast:  "log message 3",
		},
		{
			name: "query, limit 3, no start (TakeLastN)",
			req: &defangv1.TailRequest{
				Services: []string{"app"},
				Limit:    3,
				Project:  "testproject",
				LogType:  uint32(logs.LogTypeRun),
			},
			numEvents: 5,
			wantCount: 3,
			wantFirst: "log message 3",
			wantLast:  "log message 5",
		},
		{
			name: "query, limit 3, with start and end",
			req: &defangv1.TailRequest{
				Services: []string{"app"},
				Since:    timestamppb.New(baseTime),
				Until:    timestamppb.New(baseTime.Add(time.Hour)),
				Limit:    3,
				Project:  "testproject",
				LogType:  uint32(logs.LogTypeRun),
			},
			numEvents: 5,
			wantCount: 3,
			wantFirst: "log message 1",
			wantLast:  "log message 3",
		},
		{
			name: "query, limit exceeds events",
			req: &defangv1.TailRequest{
				Services: []string{"app"},
				Since:    timestamppb.New(baseTime),
				Limit:    10,
				Project:  "testproject",
				LogType:  uint32(logs.LogTypeRun),
			},
			numEvents: 3,
			wantCount: 3,
			wantFirst: "log message 1",
			wantLast:  "log message 3",
		},
		{
			name: "query, zero events",
			req: &defangv1.TailRequest{
				Services: []string{"app"},
				Since:    timestamppb.New(baseTime),
				Project:  "testproject",
				LogType:  uint32(logs.LogTypeRun),
			},
			numEvents: 0,
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newTestByocAws()
			mock := &mockCWClient{
				events: makeMockEvents(tt.numEvents, "app", etag),
			}

			logSeq, err := b.queryLogs(t.Context(), mock, tt.req)
			require.NoError(t, err)

			events := collectEvents(t, logSeq)
			assert.Len(t, events, tt.wantCount)

			if tt.wantCount > 0 {
				assert.Equal(t, tt.wantFirst, *events[0].Message)
				assert.Equal(t, tt.wantLast, *events[len(events)-1].Message)

				// Verify ascending timestamp order
				for i := 1; i < len(events); i++ {
					assert.LessOrEqual(t, *events[i-1].Timestamp, *events[i].Timestamp, "events not in ascending order at index %d", i)
				}
			}
		})
	}
}

func TestQueryLogs_FollowMode(t *testing.T) {
	b := newTestByocAws()
	mock := &mockCWClient{
		events: makeMockEvents(3, "app", "hg2xsgvsldqk"),
	}

	ctx, cancel := context.WithCancel(t.Context())
	req := &defangv1.TailRequest{
		Services: []string{"app"},
		Follow:   true,
		Project:  "testproject",
		LogType:  uint32(logs.LogTypeRun),
	}

	logSeq, err := b.queryLogs(ctx, mock, req)
	require.NoError(t, err)

	// Cancel immediately to stop the polling/tailing
	cancel()

	// Should not panic; may yield context.Canceled errors
	for _, err := range logSeq {
		if err != nil {
			assert.ErrorIs(t, err, context.Canceled)
		}
	}
}

func TestQueryCdLogs(t *testing.T) {
	baseTime := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	taskID := "abc123def456"

	tests := []struct {
		name      string
		req       *defangv1.TailRequest
		numEvents int
		wantCount int
	}{
		{
			name: "query mode, no limit",
			req: &defangv1.TailRequest{
				Etag:  taskID,
				Since: timestamppb.New(baseTime),
			},
			numEvents: 5,
			wantCount: 5,
		},
		{
			name: "query mode, with limit",
			req: &defangv1.TailRequest{
				Etag:  taskID,
				Since: timestamppb.New(baseTime),
				Limit: 3,
			},
			numEvents: 5,
			wantCount: 3,
		},
		{
			name: "query mode, with start and end",
			req: &defangv1.TailRequest{
				Etag:  taskID,
				Since: timestamppb.New(baseTime),
				Until: timestamppb.New(baseTime.Add(time.Hour)),
			},
			numEvents: 5,
			wantCount: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newTestByocAws()
			mock := &mockCWClient{
				events: makeMockEvents(tt.numEvents, "crun", ""),
			}

			batchSeq, err := b.queryCdLogs(t.Context(), mock, tt.req)
			require.NoError(t, err)

			// Flatten and collect
			logSeq := cw.Flatten(batchSeq)
			events := collectEvents(t, logSeq)
			assert.Len(t, events, tt.wantCount)
		})
	}
}

// TestQueryCdLogs_FollowMode is skipped because TailTaskID polls getTaskStatus
// (real AWS ECS API) when StartLiveTail returns ResourceNotFoundException.
// Testing follow mode for CD logs requires mocking the ECS DescribeTasks API.
func TestQueryCdLogs_FollowMode(t *testing.T) {
	t.Skip("requires ECS API mock for getTaskStatus")
}
