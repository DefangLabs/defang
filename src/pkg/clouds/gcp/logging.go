package gcp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	logging "cloud.google.com/go/logging/apiv2"
	"cloud.google.com/go/logging/apiv2/loggingpb"
	"github.com/DefangLabs/defang/src/pkg/term"
	"google.golang.org/api/iterator"
)

func (gcp Gcp) NewTailer(ctx context.Context) (Tailer, error) {
	term.Debugf("[gcp read-req] NewTailer: opening TailLogEntries stream")
	client, err := logging.NewClient(ctx, gcp.Options...)
	if err != nil {
		return nil, err
	}
	tleClient, err := client.TailLogEntries(ctx)
	if err != nil {
		return nil, err
	}
	t := &gcpLoggingTailer{
		projectId: gcp.ProjectId,
		tleClient: tleClient,
		client:    client,
	}
	return t, nil
}

type Tailer interface {
	Start(ctx context.Context, query string) error
	Next(ctx context.Context) (*loggingpb.LogEntry, error)
	Close() error
}

type gcpLoggingTailer struct {
	projectId string
	tleClient loggingpb.LoggingServiceV2_TailLogEntriesClient
	client    *logging.Client

	cache     []*loggingpb.LogEntry
	recvCount int
	recvStart time.Time
}

func (t *gcpLoggingTailer) Start(ctx context.Context, query string) error {
	term.Debugf("[gcp read-req] TailLogEntries.Send (Start)")
	req := &loggingpb.TailLogEntriesRequest{
		ResourceNames: []string{"projects/" + t.projectId},
		Filter:        query,
	}
	if err := t.tleClient.Send(req); err != nil {
		return fmt.Errorf("failed to send tail log entries request: %w", err)
	}
	return nil
}

func (t *gcpLoggingTailer) Next(ctx context.Context) (*loggingpb.LogEntry, error) {
	if len(t.cache) == 0 {
		if t.recvStart.IsZero() {
			t.recvStart = time.Now()
		}
		t.recvCount++
		term.Debugf("[gcp read-req] TailLogEntries.Recv #%d (%.1fs since first recv)", t.recvCount, time.Since(t.recvStart).Seconds())
		resp, err := t.tleClient.Recv()
		if err != nil {
			return nil, err
		}
		t.cache = resp.GetEntries()
		if len(t.cache) == 0 {
			// GCP may send empty responses (heartbeats, suppression info); return nil
			// so the caller can continue looping without treating this as an error.
			return nil, nil
		}
	}

	entry := t.cache[0]
	t.cache = t.cache[1:]
	return entry, nil
}

func (t *gcpLoggingTailer) Close() error {
	// TODO: find out how to properly close the client
	term.Debugf("Closing log tailer")
	if t.recvCount > 0 {
		elapsed := time.Since(t.recvStart)
		term.Debugf("[gcp read-req] TailLogEntries closed: %d Recv calls over %.1fs (%.1f/min)",
			t.recvCount, elapsed.Seconds(), float64(t.recvCount)/elapsed.Minutes())
	}
	e1 := t.tleClient.CloseSend()
	term.Debugf("Closing log tailer client")
	e2 := t.client.Close()
	return errors.Join(e1, e2)
}

type Lister interface {
	Next() (*loggingpb.LogEntry, error)
}

type gcpLoggingLister struct {
	it        *logging.LogEntryIterator
	client    *logging.Client
	lastToken string
	pages     int
	entries   int
}

type Order string

const (
	OrderDescending Order = "desc"
	OrderAscending  Order = "asc"
)

func (gcp Gcp) ListLogEntries(ctx context.Context, query string, order Order) (Lister, error) {
	term.Debugf("[gcp read-req] ListLogEntries (order=%s)", order)
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
	return &gcpLoggingLister{it: it, client: client}, nil
}

func (l *gcpLoggingLister) Next() (*loggingpb.LogEntry, error) {
	// Detect page fetches: the iterator's NextPageToken changes each time a new
	// page is fetched from the API (each page = one read request against quota).
	if token := l.it.PageInfo().Token; token != l.lastToken {
		l.lastToken = token
		l.pages++
		term.Debugf("[gcp read-req] ListLogEntries page fetch #%d (entries so far: %d)", l.pages, l.entries)
	}
	entry, err := l.it.Next()
	if err == iterator.Done {
		term.Debugf("[gcp read-req] ListLogEntries done: %d page(s), %d entries total", l.pages, l.entries)
		term.Debugf("Closing log lister client")
		if err := l.client.Close(); err != nil {
			return nil, err
		}
		return nil, io.EOF
	}
	if err != nil {
		return nil, err
	}
	l.entries++
	return entry, nil
}
