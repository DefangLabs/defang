package gcp

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"testing"
	"time"

	"cloud.google.com/go/logging/apiv2/loggingpb"
	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/clouds/gcp"
	"github.com/DefangLabs/defang/src/pkg/logs"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestSetUpCD(t *testing.T) {
	t.Skip("skipping test")
	ctx := t.Context()
	b := NewByocProvider(ctx, "testTenantID", "")
	account, err := b.AccountInfo(ctx)
	if err != nil {
		t.Errorf("AccountInfo() error = %v, want nil", err)
	}
	t.Logf("account: %+v", account)
	if err := b.SetUpCD(ctx); err != nil {
		t.Errorf("SetUpCD() error = %v, want nil", err)
	}

	payload := base64.StdEncoding.EncodeToString([]byte(`services:
  nginx:
    image: nginx:1-alpine
    ports:
      - "8080:80"
`))
	cmd := cdCommand{
		project: "testproj",
		command: []string{"up", payload},
	}

	if err := b.runCdCommand(ctx, cmd); err != nil {
		t.Errorf("CdCommand() error = %v, want nil", err)
	}
}

type MockGcpLogsClient struct {
	lister gcp.Lister
	tailer gcp.Tailer
}

func (m MockGcpLogsClient) ListLogEntries(ctx context.Context, query string, order gcp.Order) (gcp.Lister, error) {
	return m.lister, nil
}

func (m MockGcpLogsClient) NewTailer(ctx context.Context) (gcp.Tailer, error) {
	return m.tailer, nil
}
func (m MockGcpLogsClient) GetExecutionEnv(ctx context.Context, executionName string) (map[string]string, error) {
	return nil, nil
}
func (m MockGcpLogsClient) GetProjectID() gcp.ProjectId {
	return "test-project"
}
func (m MockGcpLogsClient) GetBuildInfo(ctx context.Context, buildId string) (*gcp.BuildTag, error) {
	return &gcp.BuildTag{
		Stack:   "test-stack",
		Project: "test-project",
		Service: "test-service",
		Etag:    "test-etag",
	}, nil
}

type MockGcpLoggingLister struct {
	logEntries []*loggingpb.LogEntry
}

func (m *MockGcpLoggingLister) Next() (*loggingpb.LogEntry, error) {
	if len(m.logEntries) > 0 {
		entry := m.logEntries[0]
		m.logEntries = m.logEntries[1:]
		return entry, nil
	}
	return nil, io.EOF
}

type MockGcpLoggingTailer struct {
	MockGcpLoggingLister
}

func (m *MockGcpLoggingTailer) Close() error {
	return nil
}

func (m *MockGcpLoggingTailer) Start(ctx context.Context, query string) error {
	return nil
}

func (m *MockGcpLoggingTailer) Next(ctx context.Context) (*loggingpb.LogEntry, error) {
	return m.MockGcpLoggingLister.Next()
}

func TestGetLogStream(t *testing.T) {
	tests := []struct {
		name        string
		req         *defangv1.TailRequest
		cdExecution string
	}{
		// TODO: use golang 1.25 synctest to avoid needing a fixed Since in every test case
		{name: "no_args", req: &defangv1.TailRequest{}},
		{name: "since", req: &defangv1.TailRequest{Since: timestamppb.New(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))}},
		{name: "since_and_until", req: &defangv1.TailRequest{
			Since: timestamppb.New(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)),
			Until: timestamppb.New(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)),
		}},
		{name: "with_pattern", req: &defangv1.TailRequest{
			Pattern: "error",
		}},
		{name: "with_pattern_since_and_until", req: &defangv1.TailRequest{
			Pattern: "error",
			Since:   timestamppb.New(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)),
			Until:   timestamppb.New(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)),
		}},
		{name: "with_project", req: &defangv1.TailRequest{
			Project: "test-project",
			LogType: uint32(logs.LogTypeAll),
		}},
		{name: "with_project_since_and_until", req: &defangv1.TailRequest{
			Project: "test-project",
			Since:   timestamppb.New(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)),
			Until:   timestamppb.New(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)),
			LogType: uint32(logs.LogTypeAll),
		}},
		{name: "with_logtype_build", req: &defangv1.TailRequest{
			LogType: uint32(logs.LogTypeBuild),
		}},
		{name: "with_logtype_run", req: &defangv1.TailRequest{
			LogType: uint32(logs.LogTypeRun),
		}},
		{name: "with_logtype_all", req: &defangv1.TailRequest{
			Pattern: "error",
			LogType: uint32(logs.LogTypeAll),
		}},
		{name: "with_etag", req: &defangv1.TailRequest{
			LogType: uint32(logs.LogTypeAll),
			Etag:    "test-etag",
		}},
		{name: "with_etag_and_since", req: &defangv1.TailRequest{
			Since:   timestamppb.New(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)),
			LogType: uint32(logs.LogTypeAll),
			Etag:    "test-etag",
		}},
		{name: "with_everything", req: &defangv1.TailRequest{
			Project: "test-project",
			Pattern: "error",
			Since:   timestamppb.New(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)),
			Until:   timestamppb.New(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)),
			LogType: uint32(logs.LogTypeAll),
			Etag:    "test-etag",
		},
			cdExecution: "test-execution-id",
		},
	}

	ctx := t.Context()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewByocProvider(ctx, "testTenantID", "")
			b.cdExecution = tt.cdExecution

			driver := &MockGcpLogsClient{
				lister: &MockGcpLoggingLister{},
				tailer: &MockGcpLoggingTailer{},
			}

			stream, err := b.getLogStream(ctx, driver, tt.req)
			if err != nil {
				t.Errorf("getLogStream() error = %v, want nil", err)
			}
			if stream == nil {
				t.Errorf("getLogStream() returned nil tailer, want non-nil")
			}

			logStream, ok := stream.(*LogStream)
			if !ok {
				t.Fatalf("getLogStream() returned wrong type, want *gcp.LogStream")
			}

			query := logStream.GetQuery()
			if err := pkg.Compare([]byte(query), "testdata/"+tt.name+".query"); err != nil {
				t.Errorf("getLogStream() query mismatch: %v", err)
			}
		})
	}
}

func TestAnnotateGcpError(t *testing.T) {
	tests := []struct {
		name              string
		err               error
		wantCredentialsErr bool
	}{
		{
			name:              "deleted project error",
			err:               &googleapi.Error{Code: 403, Message: "Project test-project has been deleted."},
			wantCredentialsErr: true,
		},
		{
			name: "USER_PROJECT_DENIED error",
			err: &googleapi.Error{
				Code:    403,
				Message: "Project access denied",
				Details: []interface{}{
					map[string]interface{}{
						"@type":  "type.googleapis.com/google.rpc.ErrorInfo",
						"reason": "USER_PROJECT_DENIED",
					},
				},
			},
			wantCredentialsErr: true,
		},
		{
			name:              "different 403 error",
			err:               &googleapi.Error{Code: 403, Message: "Access denied for resource"},
			wantCredentialsErr: false,
		},
		{
			name:              "404 error",
			err:               &googleapi.Error{Code: 404, Message: "Not found"},
			wantCredentialsErr: false,
		},
		{
			name:              "non-googleapi error",
			err:               errors.New("some other error"),
			wantCredentialsErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := annotateGcpError(tt.err)

			var credErr *CredentialsError
			gotCredentialsErr := errors.As(result, &credErr)

			if gotCredentialsErr != tt.wantCredentialsErr {
				t.Errorf("annotateGcpError() returned CredentialsError = %v, want %v", gotCredentialsErr, tt.wantCredentialsErr)
			}
		})
	}
}

func TestIsADCRefreshNeeded(t *testing.T) {
	tests := []struct {
		name string
		err  *googleapi.Error
		want bool
	}{
		{
			name: "deleted project message",
			err:  &googleapi.Error{Code: 403, Message: "Project test-project has been deleted."},
			want: true,
		},
		{
			name: "project deleted in message",
			err:  &googleapi.Error{Code: 403, Message: "The project 'test' has been deleted"},
			want: true,
		},
		{
			name: "should not match - unrelated message with project and deleted",
			err:  &googleapi.Error{Code: 403, Message: "deleted user project access denied"},
			want: false,
		},
		{
			name: "should not match - has been deleted without project",
			err:  &googleapi.Error{Code: 403, Message: "The resource has been deleted"},
			want: false,
		},
		{
			name: "should not match - words in wrong order",
			err:  &googleapi.Error{Code: 403, Message: "This has been deleted from the project"},
			want: false,
		},
		{
			name: "USER_PROJECT_DENIED reason",
			err: &googleapi.Error{
				Code:    403,
				Message: "Access denied",
				Details: []interface{}{
					map[string]interface{}{
						"reason": "USER_PROJECT_DENIED",
					},
				},
			},
			want: true,
		},
		{
			name: "403 without project issue",
			err:  &googleapi.Error{Code: 403, Message: "Access denied"},
			want: false,
		},
		{
			name: "404 error",
			err:  &googleapi.Error{Code: 404, Message: "Not found"},
			want: false,
		},
		{
			name: "500 error",
			err:  &googleapi.Error{Code: 500, Message: "Internal server error"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isADCRefreshNeeded(tt.err)
			if got != tt.want {
				t.Errorf("isADCRefreshNeeded() = %v, want %v", got, tt.want)
			}
		})
	}
}
