package gcp

import (
	"context"
	"errors"
	"fmt"
	"iter"

	logging "cloud.google.com/go/logging/apiv2"
	"cloud.google.com/go/logging/apiv2/loggingpb"
	"github.com/DefangLabs/defang/src/pkg/term"
	"google.golang.org/api/iterator"
)

type Order string

const (
	OrderDescending Order = "desc"
	OrderAscending  Order = "asc"
)

// ListLogEntries returns an iterator over log entries matching the query.
// The underlying client is closed when iteration completes or is stopped.
func (gcp Gcp) ListLogEntries(ctx context.Context, query string, order Order) (iter.Seq2[[]*loggingpb.LogEntry, error], error) {
	client, err := logging.NewClient(ctx)
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

// TailLogEntries establishes a log tail stream and sends the filter request eagerly.
// The returned iterator yields log entries as they arrive. The underlying stream
// and client are closed when iteration completes or is stopped.
func (gcp Gcp) TailLogEntries(ctx context.Context, query string) (iter.Seq2[[]*loggingpb.LogEntry, error], error) {
	client, err := logging.NewClient(ctx)
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
		for {
			resp, err := tleClient.Recv()
			if !yield(resp.GetEntries(), err) {
				return
			}
		}
	}, nil
}
