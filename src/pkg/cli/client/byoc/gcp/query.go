package gcp

import (
	"fmt"
	"strings"
	"time"

	"github.com/DefangLabs/defang/src/pkg/clouds/gcp"
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

func NewLogQuery(projectId gcp.ProjectId) *Query {
	return NewQuery(fmt.Sprintf(`(
logName=~"logs/run.googleapis.com%%2F(stdout|stderr)$" OR
logName="projects/%[1]s/logs/cloudbuild" OR
logName="projects/%[1]s/logs/cos_containers" OR
logName="projects/%[1]s/logs/docker-logs"
)`, projectId))
}

func NewSubscribeQuery() *Query {
	return NewQuery(`(
protoPayload.serviceName="run.googleapis.com" OR
protoPayload.serviceName="compute.googleapis.com" OR
protoPayload.serviceName="cloudbuild.googleapis.com"
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

func (q *Query) AddJobLogQuery(stack, project, etag string, services []string) {
	query := `resource.type = "cloud_run_job"`

	if stack != "" {
		query += fmt.Sprintf(`
labels."defang-stack" = %q`, gcp.SafeLabelValue(stack))
	}

	if project != "" {
		query += fmt.Sprintf(`
labels."defang-project" = %q`, gcp.SafeLabelValue(project))
	}

	if etag != "" {
		query += fmt.Sprintf(`
labels."defang-etag" = %q`, gcp.SafeLabelValue(etag))
	}

	if len(services) > 0 {
		query += fmt.Sprintf(`
labels."defang-service" =~ "^(%v)$"`, servicesPattern(services))
	}

	q.AddQuery(query)
}

func (q *Query) AddServiceLogQuery(stack, project, etag string, services []string) {
	query := `resource.type="cloud_run_revision"`

	if stack != "" {
		query += fmt.Sprintf(`
labels."defang-stack" = %q`, gcp.SafeLabelValue(stack))
	}

	if project != "" {
		query += fmt.Sprintf(`
labels."defang-project" = %q`, gcp.SafeLabelValue(project))
	}

	if etag != "" {
		query += fmt.Sprintf(`
labels."defang-etag" = %q`, gcp.SafeLabelValue(etag))
	}

	if len(services) > 0 {
		query += fmt.Sprintf(`
labels."defang-service" =~ "^(%v)$"`, servicesPattern(services))
	}

	q.AddQuery(query)
}

func (q *Query) AddComputeEngineLogQuery(stack, project, etag string, services []string) {
	query := `resource.type="gce_instance"`

	if stack != "" {
		query += fmt.Sprintf(`
labels."defang-stack" = %q`, gcp.SafeLabelValue(stack))
	}

	if project != "" {
		query += fmt.Sprintf(`
labels."defang-project" = %q`, gcp.SafeLabelValue(project))
	}

	if etag != "" {
		query += fmt.Sprintf(`
labels."defang-etag" = %q`, gcp.SafeLabelValue(etag))
	}

	if len(services) > 0 {
		query += fmt.Sprintf(`
labels."defang-service" =~ "^(%v)$"`, servicesPattern(services))
	}

	q.AddQuery(query)
}

func (q *Query) AddCloudBuildLogQuery(stack, project, etag string, services []string) {
	query := `resource.type="build"`

	// FIXME: Support stack
	servicesRegex := `[a-zA-Z0-9-_]{1,63}`
	if len(services) > 0 {
		servicesRegex = fmt.Sprintf("(%v)", strings.Join(services, "|")) // Cloud build labels allows upper case letters
	}
	query += fmt.Sprintf(`
labels.build_tags =~ "%v_%v_%v_%v"`, stack, project, servicesRegex, etag)
	query += `
-labels.build_step="MAIN"` // Exclude main build step logs (like "FETCHSOURCE"/"PUSH"/"DONE") to reduce noise

	q.AddQuery(query)
}

func (q *Query) AddCloudBuildActivityQuery() {
	query := `resource.type="build"
logName=~"logs/cloudaudit.googleapis.com%2Factivity$"`
	q.AddQuery(query)
}

func (q *Query) AddJobExecutionUpdateQuery(executionName string) {
	if executionName == "" {
		return
	}
	q.AddQuery(fmt.Sprintf(`labels."run.googleapis.com/execution_name" = %q`, gcp.SafeLabelValue(executionName)))
}

func (q *Query) AddJobStatusUpdateRequestQuery(stack, project, etag string, services []string) {
	reqQuery := `protoPayload.methodName="google.cloud.run.v2.Jobs.UpdateJob" OR "google.cloud.run.v2.Jobs.CreateJob"`

	if stack != "" {
		reqQuery += fmt.Sprintf(`
protoPayload.request.job.template.labels."defang-stack"=%q`, gcp.SafeLabelValue(stack))
	}

	if project != "" {
		reqQuery += fmt.Sprintf(`
protoPayload.request.job.template.labels."defang-project"=%q`, gcp.SafeLabelValue(project))
	}

	if etag != "" {
		reqQuery += fmt.Sprintf(`
protoPayload.request.job.template.labels."defang-etag"=%q`, gcp.SafeLabelValue(etag))
	}

	if len(services) > 0 {
		reqQuery += fmt.Sprintf(`
protoPayload.request.job.template.labels."defang-service"=~"^(%v)$"`, servicesPattern(services))
	}

	q.AddQuery(reqQuery)
}

func (q *Query) AddJobStatusUpdateResponseQuery(stack, project, etag string, services []string) {
	resQuery := `protoPayload.methodName="/Jobs.RunJob" OR "/Jobs.CreateJob" OR "/Jobs.UpdateJob"`

	if stack != "" {
		resQuery += fmt.Sprintf(`
protoPayload.response.spec.template.metadata.labels."defang-stack"=%q`, gcp.SafeLabelValue(stack))
	}

	if project != "" {
		resQuery += fmt.Sprintf(`
protoPayload.response.spec.template.metadata.labels."defang-project"=%q`, gcp.SafeLabelValue(project))
	}

	if etag != "" {
		resQuery += fmt.Sprintf(`
protoPayload.response.spec.template.metadata.labels."defang-etag"=%q`, gcp.SafeLabelValue(etag))
	}

	if len(services) > 0 {
		resQuery += fmt.Sprintf(`
protoPayload.response.spec.template.metadata.labels."defang-service"=~"^(%v)$"`, servicesPattern(services))
	}

	q.AddQuery(resQuery)
}

func (q *Query) AddServiceStatusRequestUpdate(stack, project, etag string, services []string) {
	reqQuery := `protoPayload.methodName="google.cloud.run.v2.Services.CreateService" OR "google.cloud.run.v2.Services.UpdateService" OR "google.cloud.run.v2.Services.ReplaceService" OR "google.cloud.run.v2.Services.DeleteService"`

	if stack != "" {
		reqQuery += fmt.Sprintf(`
protoPayload.request.service.template.labels."defang-stack"=%q`, gcp.SafeLabelValue(stack))
	}

	if project != "" {
		reqQuery += fmt.Sprintf(`
protoPayload.request.service.template.labels."defang-project"=%q`, gcp.SafeLabelValue(project))
	}

	if etag != "" {
		reqQuery += fmt.Sprintf(`
protoPayload.request.service.template.labels."defang-etag"=%q`, gcp.SafeLabelValue(etag))
	}

	if len(services) > 0 {
		reqQuery += fmt.Sprintf(`
protoPayload.request.service.template.labels."defang-service"=~"^(%v)$"`, servicesPattern(services))
	}

	q.AddQuery(reqQuery)
}

func (q *Query) AddServiceStatusReponseUpdate(stack, project, etag string, services []string) {
	resQuery := `protoPayload.methodName="/Services.CreateService" OR "/Services.UpdateService" OR "/Services.ReplaceService" OR "/Services.DeleteService"`

	if stack != "" {
		resQuery += fmt.Sprintf(`
protoPayload.response.spec.template.metadata.labels."defang-stack"=%q`, gcp.SafeLabelValue(stack))
	}

	if project != "" {
		resQuery += fmt.Sprintf(`
protoPayload.response.spec.template.metadata.labels."defang-project"=%q`, gcp.SafeLabelValue(project))
	}

	if etag != "" {
		resQuery += fmt.Sprintf(`
protoPayload.response.spec.template.metadata.labels."defang-etag"=%q`, gcp.SafeLabelValue(etag))
	}

	if len(services) > 0 {
		resQuery += fmt.Sprintf(`
protoPayload.response.spec.template.metadata.labels."defang-service"=~"^(%v)$"`, servicesPattern(services))
	}

	q.AddQuery(resQuery)
}

func (q *Query) AddComputeEngineInstanceGroupInsertOrPatch(stack, project, etag string, services []string) {
	query := `protoPayload.methodName=~"beta.compute.regionInstanceGroupManagers.(insert|patch)" AND operation.first="true"`

	if stack != "" {
		query += fmt.Sprintf(`
protoPayload.request.allInstancesConfig.properties.labels.key="defang-stack"
protoPayload.request.allInstancesConfig.properties.labels.value="%v"`, gcp.SafeLabelValue(stack))
	}

	if project != "" {
		query += fmt.Sprintf(`
protoPayload.request.allInstancesConfig.properties.labels.key="defang-project"
protoPayload.request.allInstancesConfig.properties.labels.value="%v"`, gcp.SafeLabelValue(project))
	}

	if etag != "" {
		query += fmt.Sprintf(`
protoPayload.request.allInstancesConfig.properties.labels.key="defang-etag"
protoPayload.request.allInstancesConfig.properties.labels.value="%v"`, gcp.SafeLabelValue(etag))
	}

	if len(services) > 0 {
		query += fmt.Sprintf(`
protoPayload.request.allInstancesConfig.properties.labels.key="defang-service"
protoPayload.request.allInstancesConfig.properties.labels.value=~"^(%v)$"`, servicesPattern(services))
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

func (q *Query) AddUntil(until time.Time) {
	if until.IsZero() || until.Unix() <= 0 {
		return
	}
	q.baseQuery += fmt.Sprintf(` AND (timestamp <= %q)`, until.UTC().Format(time.RFC3339Nano))
}

func (q *Query) AddFilter(filter string) {
	if filter == "" {
		return
	}
	q.baseQuery += fmt.Sprintf(` AND (%q)`, filter)
}

func servicesPattern(services []string) string {
	if len(services) == 0 {
		return ""
	}
	labelSafeServices := make([]string, len(services))
	for i, service := range services {
		labelSafeServices[i] = gcp.SafeLabelValue(service)
	}
	return strings.Join(labelSafeServices, "|")
}
