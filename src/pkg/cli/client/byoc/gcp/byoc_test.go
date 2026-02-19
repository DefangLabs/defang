package gcp

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"testing"
	"time"

	"cloud.google.com/go/logging/apiv2/loggingpb"
	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc"
	"github.com/DefangLabs/defang/src/pkg/clouds/gcp"
	"github.com/DefangLabs/defang/src/pkg/logs"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
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

func TestGetGcpProjectID(t *testing.T) {
	tests := []struct {
		name     string
		envVars  map[string]string
		expected string
	}{
		{
			name:     "No environment variables set",
			envVars:  map[string]string{},
			expected: "",
		},
		{
			name:     "GCP_PROJECT_ID is set (backward compatibility)",
			envVars:  map[string]string{"GCP_PROJECT_ID": "my-gcp-project"},
			expected: "my-gcp-project",
		},
		{
			name:     "GOOGLE_PROJECT is set",
			envVars:  map[string]string{"GOOGLE_PROJECT": "google-project"},
			expected: "google-project",
		},
		{
			name:     "GOOGLE_CLOUD_PROJECT is set",
			envVars:  map[string]string{"GOOGLE_CLOUD_PROJECT": "google-cloud-project"},
			expected: "google-cloud-project",
		},
		{
			name:     "GCLOUD_PROJECT is set",
			envVars:  map[string]string{"GCLOUD_PROJECT": "gcloud-project"},
			expected: "gcloud-project",
		},
		{
			name:     "CLOUDSDK_CORE_PROJECT is set",
			envVars:  map[string]string{"CLOUDSDK_CORE_PROJECT": "cloudsdk-project"},
			expected: "cloudsdk-project",
		},
		{
			name: "Multiple env vars set, GCP_PROJECT_ID takes precedence",
			envVars: map[string]string{
				"GCP_PROJECT_ID":        "gcp-project",
				"GOOGLE_PROJECT":        "google-project",
				"CLOUDSDK_CORE_PROJECT": "cloudsdk-project",
			},
			expected: "gcp-project",
		},
		{
			name: "Multiple env vars set, GOOGLE_PROJECT takes precedence when GCP_PROJECT_ID is not set",
			envVars: map[string]string{
				"GOOGLE_PROJECT":        "google-project",
				"GOOGLE_CLOUD_PROJECT":  "google-cloud-project",
				"CLOUDSDK_CORE_PROJECT": "cloudsdk-project",
			},
			expected: "google-project",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, envVar := range pkg.GCPProjectEnvVars {
				t.Setenv(envVar, "") // this ensures env var is restored after the test
				os.Unsetenv(envVar)  // after t.Setenv, to really unset it
			}

			// Set test environment variables
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}

			got := getGcpProjectID()
			if got != tt.expected {
				t.Errorf("getGcpProjectID() = %v, want %v", got, tt.expected)
			}
		})
	}
}

type mockGcpDriver struct {
	gcpDriver
	bucketName                             string
	getBucketWithPrefixError               error
	getBucketObjectWithServiceAccountError error
}

func (m mockGcpDriver) GetBucketWithPrefix(ctx context.Context, prefix string) (string, error) {
	return m.bucketName, m.getBucketWithPrefixError
}

func (m mockGcpDriver) GetBucketObjectWithServiceAccount(ctx context.Context, bucketName, objectName, serviceAccount string) ([]byte, error) {
	if bucketName != m.bucketName || objectName != "projects/project1/stack1/project.pb" {
		return nil, gcp.ErrObjectNotExist
	}
	projectUpdate := defangv1.ProjectUpdate{
		Services: []*defangv1.ServiceInfo{{Service: &defangv1.Service{Name: "service1"}}},
	}
	projectUpdateBytes, _ := proto.Marshal(&projectUpdate)
	return projectUpdateBytes, m.getBucketObjectWithServiceAccountError
}

func (mockGcpDriver) GetServiceAccountEmail(name string) string {
	return "mock-sa"
}

func TestGetServices(t *testing.T) {
	b := &ByocGcp{driver: mockGcpDriver{}}
	b.ByocBaseClient = byoc.NewByocBaseClient("tenant1", b, "stack1")

	t.Run("no bucket = no services", func(t *testing.T) {
		res, err := b.GetServices(t.Context(), &defangv1.GetServicesRequest{
			Project: "project1",
		})
		require.NoError(t, err)
		assert.Empty(t, res.Services)
	})

	t.Run("bucket error = error", func(t *testing.T) {
		storageErr := errors.New("storage: error")
		b.driver = &mockGcpDriver{
			getBucketWithPrefixError: storageErr,
		}
		res, err := b.GetServices(t.Context(), &defangv1.GetServicesRequest{
			Project: "project1",
		})
		require.ErrorIs(t, err, storageErr)
		assert.Nil(t, res)
	})

	t.Run("no object = no services", func(t *testing.T) {
		b.driver = &mockGcpDriver{
			bucketName:                             "bucket-a",
			getBucketObjectWithServiceAccountError: gcp.ErrObjectNotExist,
		}
		res, err := b.GetServices(t.Context(), &defangv1.GetServicesRequest{
			Project: "project1",
		})
		require.NoError(t, err)
		assert.Empty(t, res.Services)
	})

	t.Run("storage error = error", func(t *testing.T) {
		storageErr := errors.New("storage: error")
		b.driver = &mockGcpDriver{
			bucketName:                             "bucket-a",
			getBucketObjectWithServiceAccountError: storageErr,
		}
		res, err := b.GetServices(t.Context(), &defangv1.GetServicesRequest{
			Project: "project1",
		})
		require.ErrorIs(t, err, storageErr)
		assert.Nil(t, res)
	})

	t.Run("success", func(t *testing.T) {
		b.driver = &mockGcpDriver{
			bucketName:                             "bucket-a",
			getBucketObjectWithServiceAccountError: nil,
		}
		res, err := b.GetServices(t.Context(), &defangv1.GetServicesRequest{
			Project: "project1",
		})
		require.NoError(t, err)
		assert.Len(t, res.Services, 1)
	})

	t.Run("wrong project = no services", func(t *testing.T) {
		b.driver = &mockGcpDriver{
			bucketName:                             "bucket-a",
			getBucketObjectWithServiceAccountError: nil,
		}
		res, err := b.GetServices(t.Context(), &defangv1.GetServicesRequest{
			Project: "project2",
		})
		require.NoError(t, err)
		assert.Empty(t, res.Services)
	})
}
