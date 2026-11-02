package gcp

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"time"

	logging "cloud.google.com/go/logging/apiv2"
	"cloud.google.com/go/logging/apiv2/loggingpb"
	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/term"
	"google.golang.org/api/iterator"
)

// tailIdleTimeout bounds how long the tailer waits for the next TailLogEntries response
// before treating the stream as stalled. A half-dead gRPC stream (e.g. a reconnect that
// lands on a connection that never delivers data) can block on Recv() forever without ever
// erroring, so without this the read loop would otherwise block until the caller's own
// context deadline, however long that is. See https://github.com/DefangLabs/defang/issues/2231.
var tailIdleTimeout = 90 * time.Second

// TailLogEntries establishes a log tail stream and sends the filter request eagerly.
// The returned iterator yields batches of log entries as they arrive; an empty batch means
// GCP sent a response with no entries (heartbeat or suppression info), which the caller can
// simply skip over. The underlying stream and client are closed when iteration completes or
// is stopped.
func (gcp Gcp) TailLogEntries(ctx context.Context, query string) (iter.Seq2[[]*loggingpb.LogEntry, error], error) {
	client, err := logging.NewClient(ctx, gcp.Options...)
	if err != nil {
		return nil, err
	}
	tleClient, err := client.TailLogEntries(ctx)
	if err != nil {
		client.Close()
		return nil, err
	}

	req := &loggingpb.TailLogEntriesRequest{
		ResourceNames: []string{"projects/" + gcp.ProjectId},
		Filter:        query,
	}
	if err := tleClient.Send(req); err != nil {
		tleClient.CloseSend()
		client.Close()
		return nil, fmt.Errorf("failed to send tail log entries request: %w", err)
	}

	return func(yield func([]*loggingpb.LogEntry, error) bool) {
		defer func() {
			term.Debugf("Closing log tailer")
			e1 := tleClient.CloseSend()
			term.Debugf("Closing log tailer client")
			e2 := client.Close()
			if err := errors.Join(e1, e2); err != nil {
				term.Debugf("Error closing log tailer: %v", err)
			}
		}()
		for entries, err := range tailEntries(ctx, tleClient) {
			if !yield(entries, err) {
				return
			}
		}
	}, nil
}

// tailEntries turns repeated Recv() calls on tleClient into a batch iterator, applying
// tailIdleTimeout to each call. An empty batch (no error) means GCP sent a response with no
// entries (heartbeat or suppression info); the caller can simply skip over it.
func tailEntries(ctx context.Context, tleClient loggingpb.LoggingServiceV2_TailLogEntriesClient) iter.Seq2[[]*loggingpb.LogEntry, error] {
	return func(yield func([]*loggingpb.LogEntry, error) bool) {
		for {
			resp, err := pkg.CallWithIdleTimeout(ctx, tailIdleTimeout, tleClient.Recv)
			if !yield(resp.GetEntries(), err) {
				return
			}
			if err != nil {
				return
			}
		}
	}
}

type Order string

const (
	OrderDescending Order = "desc"
	OrderAscending  Order = "asc"
)

// ListLogEntries returns an iterator over log entries matching the query, yielded one at a
// time in a single-element batch to match the TailLogEntries batch shape. The underlying
// client is closed when iteration completes or is stopped.
func (gcp Gcp) ListLogEntries(ctx context.Context, query string, order Order) (iter.Seq2[[]*loggingpb.LogEntry, error], error) {
	client, err := logging.NewClient(ctx, gcp.Options...)
	if err != nil {
		return nil, err
	}

	req := &loggingpb.ListLogEntriesRequest{
		ResourceNames: []string{"projects/" + gcp.ProjectId},
		Filter:        query,
		OrderBy:       fmt.Sprintf("timestamp %s", order),
	}
	it := client.ListLogEntries(ctx, req)
	return func(yield func([]*loggingpb.LogEntry, error) bool) {
		defer func() {
			term.Debugf("Closing log lister client")
			client.Close()
		}()
		for {
			entry, err := it.Next()
			if err == iterator.Done {
				return
			}
			if err != nil {
				yield(nil, err)
				return
			}
			if !yield([]*loggingpb.LogEntry{entry}, nil) {
				return
			}
		}
	}, nil
}
