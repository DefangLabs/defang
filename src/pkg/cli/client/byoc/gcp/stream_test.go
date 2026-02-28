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

func newTestStream(t *testing.T, listEntries, tailEntries []*loggingpb.LogEntry) *ServerStream[defangv1.TailResponse] {
	t.Helper()
	ctx := t.Context()
	projectId := gcp.ProjectId("test-project-12345")
	services := []string{}
	restoreServiceName := getServiceNameRestorer(services, gcp.SafeLabelValue,
		func(entry *defangv1.TailResponse) string { return entry.Service },
		func(entry *defangv1.TailResponse, name string) *defangv1.TailResponse {
			entry.Service = name
			return entry
		})

	mockClient := &MockGcpLogsClient{
		listEntries: listEntries,
		tailEntries: tailEntries,
	}

	stream := NewServerStream(
		mockClient,
		getLogEntryParser(ctx, mockClient),
		restoreServiceName,
	)
	stream.query = NewLogQuery(projectId)
	return stream
}

func collectMessages(t *testing.T, logs iter.Seq2[*defangv1.TailResponse, error]) []string {
	t.Helper()
	var msgs []string
	for response, err := range logs {
		assert.NoError(t, err)
		if err != nil {
			break
		}
		msgs = append(msgs, response.Entries[0].Message)
	}
	return msgs
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
			logEntries := makeMockLogEntries(tt.streamSize)

			// Reverse log entries for tail direction to simulate descending order
			if tt.direction == tail {
				for i, j := 0, len(logEntries)-1; i < j; i, j = i+1, j-1 {
					logEntries[i], logEntries[j] = logEntries[j], logEntries[i]
				}
			}

			stream := newTestStream(t, logEntries, nil)

			var logs iter.Seq2[*defangv1.TailResponse, error]
			if tt.direction == head {
				logs = stream.Head(t.Context(), tt.limit)
			} else {
				logs = stream.Tail(t.Context(), tt.limit)
			}

			collectedMessages := collectMessages(t, logs)
			assert.Equal(t, tt.expectedMsgs, collectedMessages)
		})
	}
}

func TestServerStream_HeadNoLimit(t *testing.T) {
	entries := makeMockLogEntries(5)
	stream := newTestStream(t, entries, nil)
	msgs := collectMessages(t, stream.Head(t.Context(), 0))
	assert.Len(t, msgs, 5)
	assert.Equal(t, "Log entry number 0", msgs[0])
	assert.Equal(t, "Log entry number 4", msgs[4])
}

func TestServerStream_TailNoLimit(t *testing.T) {
	// Descending order entries (4,3,2,1,0)
	entries := makeMockLogEntries(5)
	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}
	stream := newTestStream(t, entries, nil)
	msgs := collectMessages(t, stream.Tail(t.Context(), 0))
	// With limit=0, no reversal — entries come in descending order
	assert.Len(t, msgs, 5)
	assert.Equal(t, "Log entry number 4", msgs[0])
	assert.Equal(t, "Log entry number 0", msgs[4])
}

func TestServerStream_EmptyStream(t *testing.T) {
	stream := newTestStream(t, nil, nil)

	t.Run("Head", func(t *testing.T) {
		msgs := collectMessages(t, stream.Head(t.Context(), 10))
		assert.Empty(t, msgs)
	})

	t.Run("Tail", func(t *testing.T) {
		msgs := collectMessages(t, stream.Tail(t.Context(), 10))
		assert.Empty(t, msgs)
	})
}

func TestServerStream_Follow(t *testing.T) {
	listEntries := makeMockLogEntries(3)
	tailEntries := []*loggingpb.LogEntry{
		{
			Payload:   &loggingpb.LogEntry_TextPayload{TextPayload: "tail entry 0"},
			Timestamp: timestamppb.Now(),
		},
		{
			Payload:   &loggingpb.LogEntry_TextPayload{TextPayload: "tail entry 1"},
			Timestamp: timestamppb.Now(),
		},
	}

	stream := newTestStream(t, listEntries, tailEntries)
	// Use a start time in the past to trigger historical listing
	logs, err := stream.Follow(t.Context(), time.Now().Add(-time.Hour))
	assert.NoError(t, err)

	msgs := collectMessages(t, logs)
	assert.Equal(t, []string{
		"Log entry number 0",
		"Log entry number 1",
		"Log entry number 2",
		"tail entry 0",
		"tail entry 1",
	}, msgs)
}

func TestServerStream_FollowNoHistory(t *testing.T) {
	tailEntries := []*loggingpb.LogEntry{
		{
			Payload:   &loggingpb.LogEntry_TextPayload{TextPayload: "tail only"},
			Timestamp: timestamppb.Now(),
		},
	}

	stream := newTestStream(t, nil, tailEntries)
	// Zero start time skips historical listing
	logs, err := stream.Follow(t.Context(), time.Time{})
	assert.NoError(t, err)

	msgs := collectMessages(t, logs)
	assert.Equal(t, []string{"tail only"}, msgs)
}

func TestServerStream_ListError(t *testing.T) {
	testErr := errors.New("list error")
	errorIter := func(yield func([]*loggingpb.LogEntry, error) bool) {
		yield(nil, testErr)
	}

	ctx := t.Context()
	mockClient := &MockGcpLogsClient{}
	stream := NewServerStream(
		mockClient,
		getLogEntryParser(ctx, mockClient),
	)
	stream.query = NewLogQuery("test-project")

	// Override the client to return an error iterator
	stream.gcpLogsClient = &errorListClient{
		MockGcpLogsClient: *mockClient,
		listIter:          errorIter,
	}

	var gotErr error
	for _, err := range stream.Head(ctx, 10) {
		if err != nil {
			gotErr = err
			break
		}
	}
	assert.ErrorIs(t, gotErr, testErr)
}

type errorListClient struct {
	MockGcpLogsClient
	listIter iter.Seq2[[]*loggingpb.LogEntry, error]
}

func (e *errorListClient) ListLogEntries(ctx context.Context, query string, order gcp.Order) (iter.Seq2[[]*loggingpb.LogEntry, error], error) {
	return e.listIter, nil
}

func TestServerStream_TailError(t *testing.T) {
	testErr := errors.New("tail error")
	errorIter := func(yield func([]*loggingpb.LogEntry, error) bool) {
		yield(nil, testErr)
	}

	ctx := t.Context()
	mockClient := &MockGcpLogsClient{}
	stream := NewServerStream(
		mockClient,
		getLogEntryParser(ctx, mockClient),
	)
	stream.query = NewLogQuery("test-project")

	stream.gcpLogsClient = &errorTailClient{
		MockGcpLogsClient: *mockClient,
		tailIter:          errorIter,
	}

	logs, err := stream.Follow(t.Context(), time.Time{})
	assert.NoError(t, err)

	var gotErr error
	for _, err := range logs {
		if err != nil {
			gotErr = err
			break
		}
	}
	assert.ErrorIs(t, gotErr, testErr)
}

type errorTailClient struct {
	MockGcpLogsClient
	tailIter iter.Seq2[[]*loggingpb.LogEntry, error]
}

func (e *errorTailClient) TailLogEntries(ctx context.Context, query string) (iter.Seq2[[]*loggingpb.LogEntry, error], error) {
	return e.tailIter, nil
}
