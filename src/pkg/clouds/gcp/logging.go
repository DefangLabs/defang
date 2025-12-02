package gcp

import (
	"context"
	"errors"
	"fmt"
	"io"

	logging "cloud.google.com/go/logging/apiv2"
	"cloud.google.com/go/logging/apiv2/loggingpb"
	"github.com/DefangLabs/defang/src/pkg/term"
	"google.golang.org/api/iterator"
)

func (gcp Gcp) NewTailer(ctx context.Context) (Tailer, error) {
	client, err := logging.NewClient(ctx)
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

	cache []*loggingpb.LogEntry
}

func (t *gcpLoggingTailer) Start(ctx context.Context, query string) error {
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
		resp, err := t.tleClient.Recv()
		if err != nil {
			return nil, err
		}
		t.cache = resp.GetEntries()
		if len(t.cache) == 0 {
			return nil, errors.New("no log entries found")
		}
	}

	entry := t.cache[0]
	t.cache = t.cache[1:]
	return entry, nil
}

func (t *gcpLoggingTailer) Close() error {
	// TODO: find out how to properly close the client
	term.Debugf("Closing log tailer")
	e1 := t.tleClient.CloseSend()
	term.Debugf("Closing log tailer client")
	e2 := t.client.Close()
	return errors.Join(e1, e2)
}

type Lister interface {
	Next() (*loggingpb.LogEntry, error)
}

type gcpLoggingLister struct {
	it     *logging.LogEntryIterator
	client *logging.Client
}

type Order string

const (
	OrderDescending Order = "desc"
	OrderAscending  Order = "asc"
)

func (gcp Gcp) ListLogEntries(ctx context.Context, query string, order Order) (Lister, error) {
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
	return &gcpLoggingLister{it: it, client: client}, nil
}

func (l *gcpLoggingLister) Next() (*loggingpb.LogEntry, error) {
	entry, err := l.it.Next()
	if err == iterator.Done {
		term.Debugf("Closing log lister client")
		if err := l.client.Close(); err != nil {
			return nil, err
		}
		return nil, io.EOF
	}
	return entry, err
}
