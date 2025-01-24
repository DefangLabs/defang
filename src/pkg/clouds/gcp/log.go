package gcp

import (
	"fmt"
	"strings"
	"time"
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
