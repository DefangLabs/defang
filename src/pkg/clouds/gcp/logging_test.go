package gcp

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"cloud.google.com/go/logging/apiv2/loggingpb"
	"github.com/DefangLabs/defang/src/pkg"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// mockTailLogEntriesClient implements loggingpb.LoggingServiceV2_TailLogEntriesClient
// for unit testing gcpLoggingTailer.Next().
type mockTailLogEntriesClient struct {
	responses []*loggingpb.TailLogEntriesResponse
	err       error
	block     chan struct{} // if non-nil, Recv blocks until this channel is closed
}

func (m *mockTailLogEntriesClient) Send(*loggingpb.TailLogEntriesRequest) error { return nil }
func (m *mockTailLogEntriesClient) Recv() (*loggingpb.TailLogEntriesResponse, error) {
	if m.block != nil {
		<-m.block
	}
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

func TestGcpLoggingTailerNext_IdleTimeout(t *testing.T) {
	// A stalled stream (Recv never returns, no error, no data) must surface
	// pkg.ErrIdleTimeout instead of blocking forever. Regression test for
	// https://github.com/DefangLabs/defang/issues/2231.
	orig := tailIdleTimeout
	tailIdleTimeout = 10 * time.Millisecond
	defer func() { tailIdleTimeout = orig }()

	client := &mockTailLogEntriesClient{block: make(chan struct{})} // never closed: Recv blocks forever
	tailer := &gcpLoggingTailer{tleClient: client}

	entry, err := tailer.Next(context.Background())
	if !errors.Is(err, pkg.ErrIdleTimeout) {
		t.Fatalf("Next() error = %v, want ErrIdleTimeout", err)
	}
	if entry != nil {
		t.Fatalf("Next() entry = %v, want nil", entry)
	}
}

func TestGcpLoggingTailerNext_ContextCanceled(t *testing.T) {
	// If ctx is canceled before the idle timeout, Next must return the context error, not
	// ErrIdleTimeout.
	orig := tailIdleTimeout
	tailIdleTimeout = time.Minute
	defer func() { tailIdleTimeout = orig }()

	client := &mockTailLogEntriesClient{block: make(chan struct{})} // never closed: Recv blocks forever
	tailer := &gcpLoggingTailer{tleClient: client}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := tailer.Next(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Next() error = %v, want context.Canceled", err)
	}
}
