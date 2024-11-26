package gcp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

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
}

func (t *Tailer) AddJobExecutionUpdate(ctx context.Context, executionId string) error {
	execFilter := fmt.Sprintf(`logName:"cloudaudit.googleapis.com"
protoPayload.serviceName="run.googleapis.com"
protoPayload.methodName="/Jobs.RunJob" OR "/Jobs.CreateJob" OR "google.cloud.run.v2.Jobs.UpdateJob" OR "google.cloud.run.v2.Jobs.CreateJob"
protoPayload.resourceName="namespaces/%v/executions/%v"`, t.projectId, executionId)
	return t.AddFilter(ctx, execFilter)
}

func (t *Tailer) AddJobStatusUpdate(ctx context.Context, project, etag string, services []string) error {
	execFilter := `logName:"cloudaudit.googleapis.com"
protoPayload.serviceName="run.googleapis.com"
protoPayload.methodName="/Jobs.RunJob" OR "/Jobs.CreateJob" OR "google.cloud.run.v2.Jobs.UpdateJob" OR "google.cloud.run.v2.Jobs.CreateJob"`

	if project != "" {
		execFilter += fmt.Sprintf(`
protoPayload.response.metadata.labels."defang-project"="%v"`, project)
	}

	if etag != "" {
		execFilter += fmt.Sprintf(`
protoPayload.response.metadata.labels."defang-etag"="%v"`, etag)
	}

	if len(services) > 0 {
		execFilter += fmt.Sprintf(`
protoPayload.response.metadata.labels."defang-service"=~"^(%v)$"`, strings.Join(services, "|"))
	}

	return t.AddFilter(ctx, execFilter)
}

func (t *Tailer) AddServiceStatusUpdate(ctx context.Context, project, etag string, services []string) error {
	serviceFilter := `logName:"cloudaudit.googleapis.com"
protoPayload.serviceName="run.googleapis.com"
protoPayload.methodName="google.cloud.run.v1.Services.CreateService" OR "/Services.CreateService" OR "/Services.ReplaceService" OR "/Services.DeleteService"`

	if project != "" {
		serviceFilter += fmt.Sprintf(`
protoPayload.response.spec.template.metadata.labels."defang-project"="%v"`, project)
	}

	if etag != "" {
		serviceFilter += fmt.Sprintf(`
protoPayload.response.spec.template.metadata.labels."defang-etag"="%v"`, etag)
	}

	if len(services) > 0 {
		serviceFilter += fmt.Sprintf(`
protoPayload.resourceName=~"^namespaces/%v/services/(%v)-[a-z0-9]{7}$"`, t.projectId, strings.Join(services, "|"))
	}

	return t.AddFilter(ctx, serviceFilter)
}

func (t *Tailer) AddJobLog(ctx context.Context, project, executionName string, services []string, since time.Time) error {
	serviceFilter := fmt.Sprintf(`resource.type = "cloud_run_job"
resource.labels.project_id = "%v"`, t.projectId)

	if executionName != "" {
		serviceFilter += fmt.Sprintf(`
labels."run.googleapis.com/execution_name" = "%v"`, executionName)
	}

	if project != "" {
		serviceFilter += fmt.Sprintf(`
labels."defang-project" = "%v"`, project)
	}

	if len(services) > 0 {
		serviceFilter += fmt.Sprintf(`
labels."defang-service" =~ "^(%v)$"`, strings.Join(services, "|"))
	}

	if !since.IsZero() {
		serviceFilter += fmt.Sprintf(`
timestamp >= "%v"`, since.Format(time.RFC3339)) // Nano?
	}

	return t.AddFilter(ctx, serviceFilter)
}

func (t *Tailer) AddServiceLog(ctx context.Context, project, etag string, services []string, since time.Time) error {
	serviceFilter := fmt.Sprintf(`resource.type="cloud_run_revision"
resource.labels.project_id="%v"`, t.projectId)

	if etag != "" {
		serviceFilter += fmt.Sprintf(`
labels."defang-etag"="%v"`, etag)
	}

	if len(services) > 0 {
		serviceFilter += fmt.Sprintf(`
resource.labels.service_name=~"^(%v)-[a-z0-9]{7}$"`, strings.Join(services, "|"))
	}

	if !since.IsZero() {
		serviceFilter += fmt.Sprintf(`
timestamp >= "%v"`, since.Format(time.RFC3339)) // Nano?
	}

	return t.AddFilter(ctx, serviceFilter)
}

func (t *Tailer) AddFilter(ctx context.Context, filter string) error {
	req := &loggingpb.TailLogEntriesRequest{
		ResourceNames: []string{"projects/" + t.projectId},
		Filter:        filter,
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
