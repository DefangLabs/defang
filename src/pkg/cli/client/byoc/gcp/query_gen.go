package gcp

import (
	"fmt"
	"strings"
	"time"
)

func CreateSinceTimestamp(since time.Time) string {
	result := ""
	if !since.IsZero() && since.Unix() > 0 {
		result = fmt.Sprintf(`
timestamp >= "%v"`, since.UTC().Format(time.RFC3339)) // Nano?
	}

	return result
}

func CreateStdQuery(projectId string) string {
	return fmt.Sprintf(`(logName=~"logs/run.googleapis.com/(stdout|stderr)$" OR logName="projects/%s/logs/cloudbuild")`, projectId)
}

func CreateJobExecutionQuery(executionName string, since time.Time) string {
	query := `resource.type = "cloud_run_job"`

	query += fmt.Sprintf(`
labels."run.googleapis.com/execution_name" = "%v"`, executionName)

	query += CreateSinceTimestamp(since)

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

	query += CreateSinceTimestamp(since)

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

	query += CreateSinceTimestamp(since)

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

	query += CreateSinceTimestamp(since)

	return query
}

func CreateJobExecutionUpdateQuery(executionName string) string {
	return fmt.Sprintf(`labels."run.googleapis.com/execution_name" = "%v"`, executionName)
}

func CreateJobStatusUpdateRequestQuery(project string, etag string, services []string) string {
	reqQuery := `protoPayload.methodName="google.cloud.run.v2.Jobs.UpdateJob" OR "google.cloud.run.v2.Jobs.CreateJob"`

	if project != "" {
		reqQuery += fmt.Sprintf(`
protoPayload.request.job.template.labels."defang-project"="%v"`, project)
	}

	if etag != "" {
		reqQuery += fmt.Sprintf(`
protoPayload.request.job.template.labels."defang-etag"="%v"`, etag)
	}

	if len(services) > 0 {
		reqQuery += fmt.Sprintf(`
protoPayload.request.job.template.labels."defang-service"=~"^(%v)$"`, strings.Join(services, "|"))
	}

	return reqQuery
}

func CreateJobStatusUpdateResponseQuery(project string, etag string, services []string) string {
	resQuery := `protoPayload.methodName="/Jobs.RunJob" OR "/Jobs.CreateJob" OR "/Jobs.UpdateJob"`

	if project != "" {
		resQuery += fmt.Sprintf(`
protoPayload.response.spec.template.metadata.labels."defang-project"="%v"`, project)
	}

	if etag != "" {
		resQuery += fmt.Sprintf(`
protoPayload.response.spec.template.metadata.labels."defang-etag"="%v"`, etag)
	}

	if len(services) > 0 {
		resQuery += fmt.Sprintf(`
protoPayload.response.spec.template.metadata.labels."defang-service"=~"^(%v)$"`, strings.Join(services, "|"))
	}

	return resQuery
}

func CreateServiceStatusRequestUpdate(project, etag string, services []string) string {
	reqQuery := `protoPayload.methodName="google.cloud.run.v2.Services.CreateService" OR "google.cloud.run.v2.Services.UpdateService"`

	if project != "" {
		reqQuery += fmt.Sprintf(`
protoPayload.request.service.template.labels."defang-service"="%v"`, project)
	}

	if etag != "" {
		reqQuery += fmt.Sprintf(`
protoPayload.request.service.template.labels."defang-etag"="%v"`, etag)
	}

	if len(services) > 0 {
		reqQuery += fmt.Sprintf(`
protoPayload.request.service.template.labels."defang-service"=~"^(%v)$"`, strings.Join(services, "|"))
	}

	return reqQuery
}

func CreateServiceStatusReponseUpdate(project, etag string, services []string) string {
	resQuery := `protoPayload.methodName="/Services.CreateService" OR "/Services.UpdateService" OR "/Services.ReplaceService" OR "/Services.DeleteService"`

	if project != "" {
		resQuery += fmt.Sprintf(`
protoPayload.response.spec.template.metadata.labels."defang-project"="%v"`, project)
	}

	if etag != "" {
		resQuery += fmt.Sprintf(`
protoPayload.response.spec.template.metadata.labels."defang-etag"="%v"`, etag)
	}

	if len(services) > 0 {
		resQuery += fmt.Sprintf(`
protoPayload.response.spec.template.metadata.labels."defang-service"=~"^(%v)$"`, strings.Join(services, "|"))
	}

	return resQuery
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
