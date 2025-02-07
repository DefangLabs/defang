package gcp

import (
	"fmt"
	"strings"
	"time"
)

type Query struct {
	baseQuery string
	queries   []string
}

func NewQuery(baseQuery string) *Query {
	return &Query{
		baseQuery: baseQuery,
	}
}

func (q *Query) AddQuery(query string) {
	q.queries = append(q.queries, query)
}

func (q *Query) GetQuery() string {
	var buf strings.Builder
	buf.WriteString(q.baseQuery)
	if len(q.queries) > 0 {
		buf.WriteString(" AND (")
		buf.WriteString(strings.Join(q.queries, "\n) OR ("))
		buf.WriteString("\n)")
	}
	return buf.String()
}

func NewLogQuery(projectId string) *Query {
	return NewQuery(fmt.Sprintf(`(logName=~"logs/run.googleapis.com/(stdout|stderr)$" OR logName="projects/%s/logs/cloudbuild")`, projectId))
}

func NewSubscribeQuery() *Query {
	return NewQuery(`protoPayload.serviceName="run.googleapis.com"`) // TODO: Add compute engine
}

func (q *Query) AddJobExecutionQuery(executionName string, since time.Time) {
	query := `resource.type = "cloud_run_job"`

	if executionName != "" {
		query += fmt.Sprintf(`
labels."run.googleapis.com/execution_name" = %q`, executionName)
	}

	query += sinceTimestamp(since)

	q.AddQuery(query)
}

func (q *Query) AddJobLogQuery(project, etag string, services []string, since time.Time) {
	query := `resource.type = "cloud_run_job"`

	if project != "" {
		query += fmt.Sprintf(`
labels."defang-project" = %q`, project)
	}

	if etag != "" {
		query += fmt.Sprintf(`
labels."defang-etag"=%q`, etag)
	}

	if len(services) > 0 {
		query += fmt.Sprintf(`
labels."defang-service" =~ "^(%v)$"`, strings.Join(services, "|"))
	}

	query += sinceTimestamp(since)

	q.AddQuery(query)
}

func (q *Query) AddServiceLogQuery(project, etag string, services []string, since time.Time) {
	query := `resource.type="cloud_run_revision"`

	if etag != "" {
		query += fmt.Sprintf(`
labels."defang-etag"=%q`, etag)
	}

	if len(services) > 0 {
		query += fmt.Sprintf(`
labels."defang-service" =~ "^(%v)$"`, strings.Join(services, "|"))
	}

	if project != "" {
		query += fmt.Sprintf(`
labels."defang-project"=%q`, project)
	}

	query += sinceTimestamp(since)

	q.AddQuery(query)
}

func (q *Query) AddCloudBuildLogQuery(project, etag string, services []string, since time.Time) {
	query := `resource.type="build"`

	servicesRegex := `[a-zA-Z0-9-]{1,63}`
	if len(services) > 0 {
		servicesRegex = fmt.Sprintf("(%v)", strings.Join(services, "|"))
	}
	query += fmt.Sprintf(`
labels.build_tags =~ "%v_%v_%v"`, project, servicesRegex, etag)

	query += sinceTimestamp(since)

	q.AddQuery(query)
}

func (q *Query) AddJobExecutionUpdateQuery(executionName string) {
	if executionName == "" {
		return
	}
	q.AddQuery(fmt.Sprintf(`labels."run.googleapis.com/execution_name" = %q`, executionName))
}

func (q *Query) AddJobStatusUpdateRequestQuery(project string, etag string, services []string) {
	reqQuery := `protoPayload.methodName="google.cloud.run.v2.Jobs.UpdateJob" OR "google.cloud.run.v2.Jobs.CreateJob"`

	if project != "" {
		reqQuery += fmt.Sprintf(`
protoPayload.request.job.template.labels."defang-project"=%q`, project)
	}

	if etag != "" {
		reqQuery += fmt.Sprintf(`
protoPayload.request.job.template.labels."defang-etag"=%q`, etag)
	}

	if len(services) > 0 {
		reqQuery += fmt.Sprintf(`
protoPayload.request.job.template.labels."defang-service"=~"^(%v)$"`, strings.Join(services, "|"))
	}

	q.AddQuery(reqQuery)
}

func (q *Query) AddJobStatusUpdateResponseQuery(project string, etag string, services []string) {
	resQuery := `protoPayload.methodName="/Jobs.RunJob" OR "/Jobs.CreateJob" OR "/Jobs.UpdateJob"`

	if project != "" {
		resQuery += fmt.Sprintf(`
protoPayload.response.spec.template.metadata.labels."defang-project"=%q`, project)
	}

	if etag != "" {
		resQuery += fmt.Sprintf(`
protoPayload.response.spec.template.metadata.labels."defang-etag"=%q`, etag)
	}

	if len(services) > 0 {
		resQuery += fmt.Sprintf(`
protoPayload.response.spec.template.metadata.labels."defang-service"=~"^(%v)$"`, strings.Join(services, "|"))
	}

	q.AddQuery(resQuery)
}

func (q *Query) AddServiceStatusRequestUpdate(project, etag string, services []string) {
	reqQuery := `protoPayload.methodName="google.cloud.run.v2.Services.CreateService" OR "google.cloud.run.v2.Services.UpdateService"`

	if project != "" {
		reqQuery += fmt.Sprintf(`
protoPayload.request.service.template.labels."defang-service"=%q`, project)
	}

	if etag != "" {
		reqQuery += fmt.Sprintf(`
protoPayload.request.service.template.labels."defang-etag"=%q`, etag)
	}

	if len(services) > 0 {
		reqQuery += fmt.Sprintf(`
protoPayload.request.service.template.labels."defang-service"=~"^(%v)$"`, strings.Join(services, "|"))
	}

	q.AddQuery(reqQuery)
}

func (q *Query) AddServiceStatusReponseUpdate(project, etag string, services []string) {
	resQuery := `protoPayload.methodName="/Services.CreateService" OR "/Services.UpdateService" OR "/Services.ReplaceService" OR "/Services.DeleteService"`

	if project != "" {
		resQuery += fmt.Sprintf(`
protoPayload.response.spec.template.metadata.labels."defang-project"=%q`, project)
	}

	if etag != "" {
		resQuery += fmt.Sprintf(`
protoPayload.response.spec.template.metadata.labels."defang-etag"=%q`, etag)
	}

	if len(services) > 0 {
		resQuery += fmt.Sprintf(`
protoPayload.response.spec.template.metadata.labels."defang-service"=~"^(%v)$"`, strings.Join(services, "|"))
	}

	q.AddQuery(resQuery)
}

func sinceTimestamp(since time.Time) string {
	result := ""
	if !since.IsZero() && since.Unix() > 0 {
		result = fmt.Sprintf(`
timestamp >= %q`, since.UTC().Format(time.RFC3339Nano))
	}

	return result
}
