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
		buf.WriteString(" AND (\n(")
		for i, query := range q.queries {
			if i > 0 {
				buf.WriteString("\n) OR (")
			}
			for _, line := range strings.Split(query, "\n") {
				buf.WriteString("\n  ")
				buf.WriteString(line)
			}
		}
		buf.WriteString("\n)\n)")
	}
	return buf.String()
}

func NewLogQuery(projectId string) *Query {
	return NewQuery(fmt.Sprintf(`(
logName=~"logs/run.googleapis.com%%2F(stdout|stderr)$" OR
logName="projects/%s/logs/cloudbuild" OR
logName="projects/%s/logs/cos_containers"
)`, projectId, projectId))
}

func NewSubscribeQuery() *Query {
	return NewQuery(`(
protoPayload.serviceName="run.googleapis.com" OR
protoPayload.serviceName="compute.googleapis.com"
)`)
}

func (q *Query) AddJobExecutionQuery(executionName string) {
	if executionName == "" {
		return
	}
	query := `resource.type = "cloud_run_job"`

	if executionName != "" {
		query += fmt.Sprintf(`
labels."run.googleapis.com/execution_name" = %q`, executionName)
	}

	q.AddQuery(query)
}

func (q *Query) AddJobLogQuery(project, etag string, services []string) {
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

	q.AddQuery(query)
}

func (q *Query) AddServiceLogQuery(project, etag string, services []string) {
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

	q.AddQuery(query)
}

func (q *Query) AddComputeEngineLogQuery(project, etag string, services []string) {
	query := `resource.type="gce_instance"`

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

	q.AddQuery(query)
}

func (q *Query) AddCloudBuildLogQuery(project, etag string, services []string) {
	query := `resource.type="build"`

	servicesRegex := `[a-zA-Z0-9-]{1,63}`
	if len(services) > 0 {
		servicesRegex = fmt.Sprintf("(%v)", strings.Join(services, "|"))
	}
	query += fmt.Sprintf(`
labels.build_tags =~ "%v_%v_%v"`, project, servicesRegex, etag)

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

func (q *Query) AddComputeEngineInstanceGroupInsertOrPatch(project, etag string, services []string) {
	query := `protoPayload.methodName=~"beta.compute.regionInstanceGroupManagers.(insert|patch)" AND operation.first="true"`

	if project != "" {
		query += fmt.Sprintf(`
protoPayload.request.allInstancesConfig.properties.labels.key="defang-project"
protoPayload.request.allInstancesConfig.properties.labels.value="%v"`, project)
	}

	if etag != "" {
		query += fmt.Sprintf(`
protoPayload.request.allInstancesConfig.properties.labels.key="defang-etag"
protoPayload.request.allInstancesConfig.properties.labels.value="%v"`, etag)
	}

	if len(services) > 0 {
		query += fmt.Sprintf(`
protoPayload.request.allInstancesConfig.properties.labels.key="defang-service"
protoPayload.request.allInstancesConfig.properties.labels.value=~"^(%v)$"`, strings.Join(services, "|"))
	}

	q.AddQuery(query)
}

func (q *Query) AddComputeEngineInstanceGroupAddInstances() {
	q.AddQuery(`protoPayload.methodName="v1.compute.instanceGroups.addInstances"`)
}

func (q *Query) AddSince(since time.Time) {
	if since.IsZero() || since.Unix() <= 0 {
		return
	}
	q.baseQuery += fmt.Sprintf(` AND (timestamp >= %q)`, since.UTC().Format(time.RFC3339Nano))
}
