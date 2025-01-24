package gcp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	logging "cloud.google.com/go/logging/apiv2"
	"cloud.google.com/go/logging/apiv2/loggingpb"
	"github.com/DefangLabs/defang/src/pkg/term"
)

func CreateStdQuery(projectId string) string {
	return fmt.Sprintf(`(logName=~"logs/run.googleapis.com/(stdout|stderr)$" OR logName="projects/%s/logs/cloudbuild")`, projectId)
}

func CreateJobExecutionQuery(executionName string, since time.Time) string {
	query := `resource.type = "cloud_run_job"`

	query += fmt.Sprintf(`
labels."run.googleapis.com/execution_name" = "%v"`, executionName)

	if !since.IsZero() && since.Unix() > 0 {
		query += fmt.Sprintf(`
timestamp >= "%v"`, since.UTC().Format(time.RFC3339)) // Nano?
	}

	return query
}

func CreateJobLogQuery(project, etag string, services []string, since time.Time) string {
	query := `resource.type = "cloud_run_job"`

	if project != "" {
		query += fmt.Sprintf(`
labels."defang-project" = "%v"`, project)
	}

	if etag != "" {
		query += fmt.Sprintf(`
labels."defang-etag"="%v"`, etag)
	}

	if len(services) > 0 {
		query += fmt.Sprintf(`
labels."defang-service" =~ "^(%v)$"`, strings.Join(services, "|"))
	}

	if !since.IsZero() && since.Unix() > 0 {
		query += fmt.Sprintf(`
timestamp >= "%v"`, since.UTC().Format(time.RFC3339)) // Nano?
	}

	return query
}

func CreateServiceLogQuery(project, etag string, services []string, since time.Time) string {
	query := `resource.type="cloud_run_revision"`

	if etag != "" {
		query += fmt.Sprintf(`
labels."defang-etag"="%v"`, etag)
	}

	if len(services) > 0 {
		query += fmt.Sprintf(`
labels."defang-service" =~ "^(%v)$"`, strings.Join(services, "|"))
	}

	if project != "" {
		query += fmt.Sprintf(`
labels."defang-project"="%v"`, project)
	}

	if !since.IsZero() && since.Unix() > 0 {
		query += fmt.Sprintf(`
timestamp >= "%v"`, since.UTC().Format(time.RFC3339)) // Nano?
	}

	return query
}

func CreateCloudBuildLogQuery(project, etag string, services []string, since time.Time) string {
	query := `resource.type="build"`

	servicesRegex := `[a-zA-Z0-9-]{1,63}`
	if len(services) > 0 {
		servicesRegex = fmt.Sprintf("(%v)", strings.Join(services, "|"))
	}
	query += fmt.Sprintf(`
labels.build_tags =~ "%v_%v_%v"`, project, servicesRegex, etag)

	if !since.IsZero() && since.Unix() > 0 {
		query += fmt.Sprintf(`
timestamp >= "%v"`, since.UTC().Format(time.RFC3339)) // Nano?
	}

	return query
}

func CreateJobExecutionUpdateQuery(executionName string) string {
	return fmt.Sprintf(`labels."run.googleapis.com/execution_name" = "%v"`, executionName)
}

func ConcatQuery(existingQuery, newQuery string) string {
	if len(existingQuery) > 0 {
		if existingQuery[len(existingQuery)-1] == ')' {
			existingQuery += " OR "
		} else {
			existingQuery += " AND "
		}
	}
	existingQuery += "(" + newQuery + "\n)"

	return existingQuery
}

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
	t.query = ConcatQuery(t.query, query)
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
	term.Debugf("Starting log tailer with query: \n%v", t.query)
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
			return nil, errors.New("no log entries found")
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
