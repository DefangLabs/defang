package gcp

import (
	"context"
	"errors"
	"iter"
	"strconv"
	"testing"
	"time"

	"cloud.google.com/go/logging/apiv2/loggingpb"
	"github.com/DefangLabs/defang/src/pkg/clouds/gcp"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	monitoredres "google.golang.org/genproto/googleapis/api/monitoredres"
	auditpb "google.golang.org/genproto/googleapis/cloud/audit"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type HasName struct {
	name string
}

func (h HasName) Name() string {
	return h.name
}
func (h *HasName) SetName(name string) {
	h.name = name
}

func TestServiceNameRestorer(t *testing.T) {
	services := []string{"service1", "Service2", "SERVICE3", "Service4️⃣", "服务五", "Ṡervicė6"}
	restorer := getServiceNameRestorer(
		services,
		gcp.SafeLabelValue,
		func(n HasName) string { return n.Name() },
		func(n HasName, name string) HasName {
			n.SetName(name)
			return n
		},
	)
	tests := []struct {
		input    string
		expected string
	}{
		{"service1", "service1"},
		{"service2", "Service2"},
		{"service3", "SERVICE3"},
		{"service4-", "Service4️⃣"},
		{"服务五", "服务五"},
		{"ṡervicė6", "Ṡervicė6"},
	}
	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			result := restorer(HasName{name: test.input})
			if result.Name() != test.expected {
				t.Errorf("Expected %s, got: %s", test.expected, result.Name())
			}
		})
	}
}
func makeMockLogEntries(n int) []*loggingpb.LogEntry {
	logEntries := make([]*loggingpb.LogEntry, n)
	for i := range logEntries {
		logEntries[i] = &loggingpb.LogEntry{
			Payload: &loggingpb.LogEntry_TextPayload{
				TextPayload: "Log entry number " + strconv.Itoa(i),
			},
			Timestamp: timestamppb.Now(),
		}
	}
	return logEntries
}

func TestServerStream_Start(t *testing.T) {
	type directionType string
	const (
		head directionType = "head"
		tail directionType = "tail"
	)

	tests := []struct {
		name         string
		direction    directionType
		limit        int32
		streamSize   int
		expectedMsgs []string
	}{
		{
			name:       "Head with limit less than stream size",
			direction:  head,
			limit:      2,
			streamSize: 3,
			expectedMsgs: []string{
				"Log entry number 0",
				"Log entry number 1",
			},
		},
		{
			name:       "Head with limit greater than stream size",
			direction:  head,
			limit:      3,
			streamSize: 2,
			expectedMsgs: []string{
				"Log entry number 0",
				"Log entry number 1",
			},
		},
		{
			name:       "Tail with limit less than stream size",
			direction:  tail,
			limit:      2,
			streamSize: 3,
			expectedMsgs: []string{
				"Log entry number 1",
				"Log entry number 2",
			},
		},
		{
			name:       "Tail with limit greater than stream size",
			direction:  tail,
			limit:      3,
			streamSize: 2,
			expectedMsgs: []string{
				"Log entry number 0",
				"Log entry number 1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			projectId := gcp.ProjectId("test-project-12345")
			services := []string{}
			restoreServiceName := getServiceNameRestorer(services, gcp.SafeLabelValue,
				func(entry *defangv1.TailResponse) string { return entry.Service },
				func(entry *defangv1.TailResponse, name string) *defangv1.TailResponse {
					entry.Service = name
					return entry
				})

			logEntries := makeMockLogEntries(tt.streamSize)

			// Reverse log entries for tail direction to simulate descending order
			if tt.direction == tail {
				for i, j := 0, len(logEntries)-1; i < j; i, j = i+1, j-1 {
					logEntries[i], logEntries[j] = logEntries[j], logEntries[i]
				}
			}

			mockGcpLogsClient := &MockGcpLogsClient{
				lister: &MockGcpLoggingLister{
					logEntries: logEntries,
				},
				tailer: &MockGcpLoggingTailer{},
			}

			stream := NewServerStream(
				ctx,
				mockGcpLogsClient,
				getLogEntryParser(ctx, mockGcpLogsClient),
				restoreServiceName,
			)
			stream.query = NewLogQuery(projectId)

			var logs iter.Seq2[*defangv1.TailResponse, error]
			if tt.direction == head {
				logs = stream.Head(tt.limit)
			} else {
				logs = stream.Tail(tt.limit)
			}

			var collectedMessages []string
			for response, err := range logs {
				assert.NoError(t, err)
				if err != nil {
					break
				}
				collectedMessages = append(collectedMessages, response.Entries[0].Message)
			}
			assert.Equal(t, len(tt.expectedMsgs), len(collectedMessages))
			assert.Equal(t, tt.expectedMsgs, collectedMessages)
		})
	}
}

// TestServerStream_Follow_SkipsNilEntries verifies that Follow() skips nil entries
// returned by the tailer (heartbeat or suppression-info responses from GCP) and
// continues yielding real log entries without error.
func TestServerStream_Follow_SkipsNilEntries(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	// instanceId avoids a nil-pointer dereference in the parser when Resource is absent.
	svcLabels := map[string]string{"defang-service": "svc", "instanceId": "inst1"}

	realEntry := &loggingpb.LogEntry{
		Payload:   &loggingpb.LogEntry_TextPayload{TextPayload: "real log"},
		Labels:    svcLabels,
		Timestamp: timestamppb.Now(),
	}

	// cancelEntry is a sentinel: when the tailer returns it, we cancel the context
	// so the Follow loop exits cleanly rather than blocking forever.
	cancelEntry := &loggingpb.LogEntry{
		Payload:   &loggingpb.LogEntry_TextPayload{TextPayload: "cancel"},
		Labels:    svcLabels,
		Timestamp: timestamppb.Now(),
	}

	tailerEntries := []*loggingpb.LogEntry{
		nil, // heartbeat — must be skipped
		realEntry,
		nil, // suppression info — must be skipped
		cancelEntry,
	}

	mockClient := &MockGcpLogsClient{
		lister: &MockGcpLoggingLister{},
		tailer: &MockGcpLoggingTailer{
			MockGcpLoggingLister: MockGcpLoggingLister{logEntries: tailerEntries},
		},
	}

	services := []string{"svc"}
	restoreServiceName := getServiceNameRestorer(services, gcp.SafeLabelValue,
		func(entry *defangv1.TailResponse) string { return entry.Service },
		func(entry *defangv1.TailResponse, name string) *defangv1.TailResponse {
			entry.Service = name
			return entry
		})

	stream := NewServerStream(ctx, mockClient, getLogEntryParser(ctx, mockClient), restoreServiceName)
	stream.query = NewLogQuery(mockClient.GetProjectID())

	seq, err := stream.Follow(time.Time{}) // zero start → skip listing, go straight to tail
	assert.NoError(t, err)

	var messages []string
	for resp, err := range seq {
		assert.NoError(t, err)
		if err != nil {
			break
		}
		msg := resp.Entries[0].Message
		messages = append(messages, msg)
		if msg == "cancel" {
			cancel()
		}
	}

	assert.Equal(t, []string{"real log", "cancel"}, messages,
		"Follow() should skip nil tailer entries and yield real entries")
}

// activityParserMock wraps MockGcpLogsClient with a configurable GetInstanceGroupManagerLabels.
type activityParserMock struct {
	MockGcpLogsClient
	labels    map[string]string
	labelsErr error
}

func (m *activityParserMock) GetInstanceGroupManagerLabels(_ context.Context, _, _, _ string) (map[string]string, error) {
	return m.labels, m.labelsErr
}

// makeAuditLogEntry builds a loggingpb.LogEntry whose payload is a marshaled auditpb.AuditLog.
func makeAuditLogEntry(resourceType string, resourceLabels, entryLabels map[string]string, auditLog *auditpb.AuditLog) *loggingpb.LogEntry {
	payload, err := anypb.New(auditLog)
	if err != nil {
		panic(err)
	}
	return &loggingpb.LogEntry{
		Payload: &loggingpb.LogEntry_ProtoPayload{ProtoPayload: payload},
		Resource: &monitoredres.MonitoredResource{
			Type:   resourceType,
			Labels: resourceLabels,
		},
		Labels: entryLabels,
	}
}

func TestActivityParser_GceInstanceGroupManager(t *testing.T) {
	tests := []struct {
		name          string
		etag          string
		labels        map[string]string
		labelsErr     error
		rootTriggerId string
		wantResp      *defangv1.SubscribeResponse
	}{
		{
			name:          "happy path",
			labels:        map[string]string{"defang-service": "my-svc", "defang-stack": "beta"},
			rootTriggerId: "trigger-abc",
			wantResp: &defangv1.SubscribeResponse{
				Name:  "my-svc",
				State: defangv1.ServiceState_DEPLOYMENT_PENDING,
			},
		},
		{
			name:          "labels API error",
			labelsErr:     errors.New("rpc error"),
			rootTriggerId: "trigger-abc",
			wantResp:      nil,
		},
		{
			name:          "nil labels (no allInstancesConfig)",
			labels:        nil,
			rootTriggerId: "trigger-abc",
			wantResp:      nil,
		},
		{
			name:          "missing defang-service label",
			labels:        map[string]string{"defang-stack": "beta"},
			rootTriggerId: "trigger-abc",
			wantResp:      nil,
		},
		{
			name:   "missing root_trigger_id still returns DEPLOYMENT_PENDING",
			labels: map[string]string{"defang-service": "my-svc"},
			wantResp: &defangv1.SubscribeResponse{
				Name:  "my-svc",
				State: defangv1.ServiceState_DEPLOYMENT_PENDING,
			},
		},
		// etag scoping tests
		{
			name:          "etag matches — accepted",
			etag:          "abc123",
			labels:        map[string]string{"defang-service": "my-svc", "defang-etag": "abc123"},
			rootTriggerId: "trigger-abc",
			wantResp: &defangv1.SubscribeResponse{
				Name:  "my-svc",
				State: defangv1.ServiceState_DEPLOYMENT_PENDING,
			},
		},
		{
			name:          "etag mismatch — skipped",
			etag:          "abc123",
			labels:        map[string]string{"defang-service": "my-svc", "defang-etag": "other-etag"},
			rootTriggerId: "trigger-abc",
			wantResp:      nil,
		},
		{
			name:          "defang-etag label missing when etag expected — skipped",
			etag:          "abc123",
			labels:        map[string]string{"defang-service": "my-svc"},
			rootTriggerId: "trigger-abc",
			wantResp:      nil,
		},
		{
			name:          "no expected etag — etag label ignored",
			etag:          "",
			labels:        map[string]string{"defang-service": "my-svc", "defang-etag": "any-etag"},
			rootTriggerId: "trigger-abc",
			wantResp: &defangv1.SubscribeResponse{
				Name:  "my-svc",
				State: defangv1.ServiceState_DEPLOYMENT_PENDING,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			mock := &activityParserMock{labels: tt.labels, labelsErr: tt.labelsErr}
			parser := getActivityParser(ctx, mock, false, tt.etag)

			entry := makeAuditLogEntry(
				"gce_instance_group_manager",
				map[string]string{
					"project_id":                  "test-project",
					"location":                    "us-central1",
					"instance_group_manager_name": "test-manager",
				},
				map[string]string{
					"compute.googleapis.com/root_trigger_id": tt.rootTriggerId,
				},
				&auditpb.AuditLog{},
			)

			resps, err := parser(entry)
			require.NoError(t, err)

			if tt.wantResp == nil {
				assert.Nil(t, resps)
			} else {
				require.Len(t, resps, 1)
				assert.Equal(t, tt.wantResp.Name, resps[0].Name)
				assert.Equal(t, tt.wantResp.State, resps[0].State)
			}
		})
	}
}

// TestActivityParser_GceInstanceGroupFlow verifies the full flow: a gce_instance_group_manager
// entry populates the root-trigger map, and a subsequent gce_instance_group addInstances entry
// uses that map to emit DEPLOYMENT_COMPLETED.
func TestActivityParser_GceInstanceGroupFlow(t *testing.T) {
	ctx := t.Context()
	const rootTriggerId = "trigger-xyz"
	const serviceName = "my-svc"

	mock := &activityParserMock{
		labels: map[string]string{"defang-service": serviceName},
	}
	parser := getActivityParser(ctx, mock, false, "")

	// First: gce_instance_group_manager entry (insert/patch) — populates trigger map
	mgrEntry := makeAuditLogEntry(
		"gce_instance_group_manager",
		map[string]string{
			"project_id":                  "test-project",
			"location":                    "us-central1",
			"instance_group_manager_name": "test-manager",
		},
		map[string]string{
			"compute.googleapis.com/root_trigger_id": rootTriggerId,
		},
		&auditpb.AuditLog{},
	)
	resps, err := parser(mgrEntry)
	require.NoError(t, err)
	require.Len(t, resps, 1)
	assert.Equal(t, serviceName, resps[0].Name)
	assert.Equal(t, defangv1.ServiceState_DEPLOYMENT_PENDING, resps[0].State)

	// Second: gce_instance_group addInstances entry — resolves via trigger map
	doneResponse, err := structpb.NewStruct(map[string]any{"status": "DONE"})
	require.NoError(t, err)
	groupEntry := makeAuditLogEntry(
		"gce_instance_group",
		map[string]string{"project_id": "test-project"},
		map[string]string{
			"compute.googleapis.com/root_trigger_id": rootTriggerId,
		},
		&auditpb.AuditLog{
			Response: doneResponse,
		},
	)
	resps, err = parser(groupEntry)
	require.NoError(t, err)
	require.Len(t, resps, 1)
	assert.Equal(t, serviceName, resps[0].Name)
	assert.Equal(t, defangv1.ServiceState_DEPLOYMENT_COMPLETED, resps[0].State)
}

// TestActivityParser_GceInstanceGroupDropsUnknownTrigger verifies that gce_instance_group
// events with an unrecognized root_trigger_id are silently dropped.
func TestActivityParser_GceInstanceGroupDropsUnknownTrigger(t *testing.T) {
	ctx := t.Context()
	mock := &activityParserMock{labels: map[string]string{"defang-service": "my-svc"}}
	parser := getActivityParser(ctx, mock, false, "")

	doneResponse, err := structpb.NewStruct(map[string]any{"status": "DONE"})
	require.NoError(t, err)
	entry := makeAuditLogEntry(
		"gce_instance_group",
		map[string]string{"project_id": "test-project"},
		map[string]string{
			"compute.googleapis.com/root_trigger_id": "unknown-trigger",
		},
		&auditpb.AuditLog{Response: doneResponse},
	)

	resps, err := parser(entry)
	require.NoError(t, err)
	assert.Nil(t, resps)
}
