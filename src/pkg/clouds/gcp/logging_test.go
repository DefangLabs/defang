package gcp

import (
	"context"
	"io"
	"testing"

	"cloud.google.com/go/logging/apiv2/loggingpb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// mockTailLogEntriesClient implements loggingpb.LoggingServiceV2_TailLogEntriesClient
// for unit testing gcpLoggingTailer.Next().
type mockTailLogEntriesClient struct {
	responses []*loggingpb.TailLogEntriesResponse
	err       error
}

func (m *mockTailLogEntriesClient) Send(*loggingpb.TailLogEntriesRequest) error { return nil }
func (m *mockTailLogEntriesClient) Recv() (*loggingpb.TailLogEntriesResponse, error) {
	if len(m.responses) == 0 {
		if m.err != nil {
			return nil, m.err
		}
		return nil, io.EOF
	}
	resp := m.responses[0]
	m.responses = m.responses[1:]
	return resp, nil
}
func (m *mockTailLogEntriesClient) Header() (metadata.MD, error) { return nil, nil }
func (m *mockTailLogEntriesClient) Trailer() metadata.MD         { return nil }
func (m *mockTailLogEntriesClient) CloseSend() error             { return nil }
func (m *mockTailLogEntriesClient) Context() context.Context     { return context.Background() }
func (m *mockTailLogEntriesClient) SendMsg(any) error            { return nil }
func (m *mockTailLogEntriesClient) RecvMsg(any) error            { return nil }

var _ grpc.ClientStream = (*mockTailLogEntriesClient)(nil)

func TestGcpLoggingTailerNext_EmptyResponse(t *testing.T) {
	// An empty-entries response (heartbeat or suppression info) must return nil, nil
	// so the caller can continue looping without treating it as an error.
	client := &mockTailLogEntriesClient{
		responses: []*loggingpb.TailLogEntriesResponse{
			{Entries: nil}, // empty — heartbeat
		},
	}
	tailer := &gcpLoggingTailer{tleClient: client}

	entry, err := tailer.Next(context.Background())
	if err != nil {
		t.Fatalf("Next() error = %v, want nil", err)
	}
	if entry != nil {
		t.Fatalf("Next() entry = %v, want nil", entry)
	}
}

func TestGcpLoggingTailerNext_WithEntries(t *testing.T) {
	// A response with entries should return the first entry and cache the rest.
	entries := []*loggingpb.LogEntry{
		{InsertId: "entry1"},
		{InsertId: "entry2"},
	}
	client := &mockTailLogEntriesClient{
		responses: []*loggingpb.TailLogEntriesResponse{
			{Entries: entries},
		},
	}
	tailer := &gcpLoggingTailer{tleClient: client}

	entry, err := tailer.Next(context.Background())
	if err != nil {
		t.Fatalf("Next() error = %v, want nil", err)
	}
	if entry == nil || entry.InsertId != "entry1" {
		t.Fatalf("Next() entry = %v, want entry1", entry)
	}

	// Second call should return cached entry without calling Recv again.
	entry, err = tailer.Next(context.Background())
	if err != nil {
		t.Fatalf("Next() error = %v, want nil", err)
	}
	if entry == nil || entry.InsertId != "entry2" {
		t.Fatalf("Next() entry = %v, want entry2", entry)
	}
}
