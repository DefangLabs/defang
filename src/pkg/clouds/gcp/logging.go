package gcp

import (
	"context"
	"errors"
	"fmt"

	logging "cloud.google.com/go/logging/apiv2"
	"cloud.google.com/go/logging/apiv2/loggingpb"
)

func (gcp Gcp) NewTailer(ctx context.Context) (*Tailer, error) {
	client, err := logging.NewClient(ctx)
	if err != nil {
		return nil, err
	}
	tleClient, err := client.TailLogEntries(ctx)
	if err != nil {
		return nil, err
	}
	t := &Tailer{
		projectId: gcp.ProjectId,
		tleClient: tleClient,
	}
	return t, nil
}

type Tailer struct {
	projectId string
	tleClient loggingpb.LoggingServiceV2_TailLogEntriesClient

	cache []*loggingpb.LogEntry
	query string
}

func (t *Tailer) SetBaseQuery(query string) {
	t.query = query
}

func (t *Tailer) AddQuerySet(query string) {
	if len(t.query) > 0 {
		if t.query[len(t.query)-1] == ')' {
			t.query += " OR "
		} else {
			t.query += " AND "
		}
	}
	t.query += "(" + query + "\n)"
}

func (t *Tailer) Start(ctx context.Context) error {
	req := &loggingpb.TailLogEntriesRequest{
		ResourceNames: []string{"projects/" + t.projectId},
		Filter:        t.query,
	}
	if err := t.tleClient.Send(req); err != nil {
		return fmt.Errorf("failed to send tail log entries request: %w", err)
	}
	return nil
}

func (t *Tailer) Next(ctx context.Context) (*loggingpb.LogEntry, error) {
	if len(t.cache) == 0 {
		resp, err := t.tleClient.Recv()
		if err != nil {
			return nil, err
		}
		t.cache = resp.GetEntries()
		if len(t.cache) == 0 {
			return nil, errors.New("No log entries found")
		}
	}

	entry := t.cache[0]
	t.cache = t.cache[1:]
	return entry, nil
}

func (t Tailer) Close() error {
	// TODO: find out how to properly close the client
	return t.tleClient.CloseSend()
}
