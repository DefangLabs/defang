package gcp

import (
	"context"
	"encoding/base64"
	"io"
	"testing"
	"time"

	"cloud.google.com/go/logging/apiv2/loggingpb"
	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/clouds/gcp"
	"github.com/DefangLabs/defang/src/pkg/logs"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
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
	if err := b.setUpCD(ctx); err != nil {
		t.Errorf("setUpCD() error = %v, want nil", err)
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

	if op, err := b.runCdCommand(ctx, cmd); err != nil {
		t.Errorf("BootstrapCommand() error = %v, want nil", err)
	} else {
		t.Logf("BootstrapCommand() = %v", op)
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
	logEntries []loggingpb.LogEntry
}

func (m *MockGcpLoggingLister) Next() (*loggingpb.LogEntry, error) {
	if len(m.logEntries) > 0 {
		entry := &m.logEntries[0]
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

func TestGetCDExecutionContext(t *testing.T) {
	tests := []struct {
		name        string
		listEntries []loggingpb.LogEntry
		tailEntries []loggingpb.LogEntry
	}{
		{name: "no entries"},
		{name: "with only list entries",
			listEntries: []loggingpb.LogEntry{
				{Payload: &loggingpb.LogEntry_TextPayload{TextPayload: "log entry 1 from lister"}},
				{Payload: &loggingpb.LogEntry_TextPayload{TextPayload: "log entry 2 from lister"}},
			},
		},
		{name: "with only tail entries",
			tailEntries: []loggingpb.LogEntry{
				{Payload: &loggingpb.LogEntry_TextPayload{TextPayload: "log entry 1 from tailer"}},
				{Payload: &loggingpb.LogEntry_TextPayload{TextPayload: "log entry 2 from tailer"}},
			},
		},
		{name: "with both list and tail entries",
			listEntries: []loggingpb.LogEntry{
				{Payload: &loggingpb.LogEntry_TextPayload{TextPayload: "log entry 1 from lister"}},
				{Payload: &loggingpb.LogEntry_TextPayload{TextPayload: "log entry 2 from lister"}},
			},
			tailEntries: []loggingpb.LogEntry{
				{Payload: &loggingpb.LogEntry_TextPayload{TextPayload: "log entry 1 from tailer"}},
				{Payload: &loggingpb.LogEntry_TextPayload{TextPayload: "log entry 2 from tailer"}},
			},
		},
	}

	ctx := t.Context()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewByocProvider(ctx, "testTenantID", "")

			driver := &MockGcpLogsClient{
				lister: &MockGcpLoggingLister{},
				tailer: &MockGcpLoggingTailer{},
			}
			newCtx, err := b.getCDExecutionContext(ctx, driver, &defangv1.TailRequest{})
			if err != nil {
				t.Errorf("getCDExecutionContext() error = %v, want nil", err)
			}
			if newCtx == ctx {
				t.Errorf("getCDExecutionContext() returned same context, want new context")
			}
			// Wait for subscription done
			select {
			case <-newCtx.Done():
			case <-time.After(10 * time.Second):
				t.Errorf("getCDExecutionContext() timeout waiting for done")
			}
		})
	}
}

func TestGetLogStream(t *testing.T) {
	tests := []struct {
		name        string
		req         *defangv1.TailRequest
		cdExecution string
	}{
		// TODO: use golang 1.25 synctest to avoid needing a fixed Since in every test case
		{name: "since", req: &defangv1.TailRequest{Since: timestamppb.New(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))}},
		{name: "since_and_until", req: &defangv1.TailRequest{
			Since: timestamppb.New(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)),
			Until: timestamppb.New(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)),
		}},
		{name: "with_pattern", req: &defangv1.TailRequest{
			Since:   timestamppb.New(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)),
			Pattern: "error",
		}},
		{name: "with_project", req: &defangv1.TailRequest{
			Since:   timestamppb.New(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)),
			Project: "test-project",
			LogType: uint32(logs.LogTypeAll),
		}},
		{name: "with_logtype_build", req: &defangv1.TailRequest{
			Since:   timestamppb.New(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)),
			LogType: uint32(logs.LogTypeBuild),
		}},
		{name: "with_logtype_run", req: &defangv1.TailRequest{
			Since:   timestamppb.New(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)),
			LogType: uint32(logs.LogTypeRun),
		}},
		{name: "with_logtype_all", req: &defangv1.TailRequest{
			Since:   timestamppb.New(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)),
			Pattern: "error",
			LogType: uint32(logs.LogTypeAll),
		}},
		{name: "with_cd_exec", req: &defangv1.TailRequest{
			Since:   timestamppb.New(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)),
			LogType: uint32(logs.LogTypeAll),
		},
			cdExecution: "test-execution-id",
		},
		{name: "with_etag", req: &defangv1.TailRequest{
			Since:   timestamppb.New(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)),
			LogType: uint32(logs.LogTypeAll),
			Etag:    "test-etag",
		}},
		{name: "with_etag_equal_cd_exec", req: &defangv1.TailRequest{
			Since:   timestamppb.New(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)),
			LogType: uint32(logs.LogTypeAll),
			Etag:    "test-execution-id",
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
