package gcp

import (
	"strconv"
	"testing"

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
				t.Errorf("Expected %s, got %s", test.expected, result.Name())
			}
		})
	}
}
func makeMockLogEntries(n int) []loggingpb.LogEntry {
	logEntries := make([]loggingpb.LogEntry, n)
	for i := range logEntries {
		logEntries[i] = loggingpb.LogEntry{
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

			stream, err := NewServerStream(
				ctx,
				mockGcpLogsClient,
				getLogEntryParser(ctx, mockGcpLogsClient),
				restoreServiceName,
			)
			assert.NoError(t, err)
			stream.query = NewLogQuery(projectId)

			if tt.direction == head {
				stream.StartHead(tt.limit)
			} else {
				stream.StartTail(tt.limit)
			}

			collectedMessages := []string{}
			for {
				if !stream.Receive() {
					assert.NoError(t, stream.Err())
					break
				}
				response := stream.Msg()
				collectedMessages = append(collectedMessages, response.Entries[0].Message)
			}
			assert.Equal(t, len(tt.expectedMsgs), len(collectedMessages))
			assert.Equal(t, tt.expectedMsgs, collectedMessages)
		})
	}
}
