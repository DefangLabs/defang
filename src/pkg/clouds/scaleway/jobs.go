package scaleway

import (
	"context"
	"fmt"
	"net/url"
)

// JobResources defines the CPU and memory for a serverless job.
type JobResources struct {
	CPULimit    int `json:"cpu_limit"`
	MemoryLimit int `json:"memory_limit"`
}

// JobDefinition represents a Scaleway Serverless Jobs definition.
type JobDefinition struct {
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	ProjectID       string            `json:"project_id"`
	CPULimit        int               `json:"cpu_limit"`
	MemoryLimit     int               `json:"memory_limit"`
	ImageURI        string            `json:"image_uri"`
	EnvironmentVars map[string]string `json:"environment_variables"`
	Region          string            `json:"region"`
	CreatedAt       string            `json:"created_at"`
}

// JobRun represents a single execution of a serverless job.
type JobRun struct {
	ID              string `json:"id"`
	JobDefinitionID string `json:"job_definition_id"`
	State           string `json:"state"`
	CreatedAt       string `json:"created_at"`
	StartedAt       string `json:"started_at,omitempty"`
	TerminatedAt    string `json:"terminated_at,omitempty"`
	ExitCode        *int   `json:"exit_code,omitempty"`
	ErrorMessage    string `json:"error_message,omitempty"`
}

type listJobRunsResponse struct {
	JobRuns    []JobRun `json:"job_runs"`
	TotalCount int      `json:"total_count"`
}

// CreateJobDefinition creates a new serverless job definition.
func (c *Client) CreateJobDefinition(ctx context.Context, name, image string, env map[string]string, resources JobResources) (*JobDefinition, error) {
	url := c.regionURL("serverless-jobs", "v1alpha1") + "/job-definitions"
	body := map[string]any{
		"name":                    name,
		"project_id":             c.ProjectID,
		"cpu_limit":              resources.CPULimit,
		"memory_limit":           resources.MemoryLimit,
		"image_uri":              image,
		"environment_variables":  env,
	}
	var def JobDefinition
	if err := c.doRequestJSON(ctx, "POST", url, body, &def); err != nil {
		return nil, AnnotateScalewayError(err, fmt.Sprintf("creating job definition %q", name))
	}
	return &def, nil
}

// RunJob starts a new run of a job definition with optional environment overrides.
func (c *Client) RunJob(ctx context.Context, definitionID string, envOverrides map[string]string) (*JobRun, error) {
	endpoint := c.regionURL("serverless-jobs", "v1alpha1") + "/job-definitions/" + definitionID + "/start"
	body := map[string]any{}
	if len(envOverrides) > 0 {
		body["environment_variables"] = envOverrides
	}
	var run JobRun
	if err := c.doRequestJSON(ctx, "POST", endpoint, body, &run); err != nil {
		return nil, AnnotateScalewayError(err, fmt.Sprintf("running job %q", definitionID))
	}
	return &run, nil
}

// GetJobRun retrieves the status of a specific job run.
func (c *Client) GetJobRun(ctx context.Context, runID string) (*JobRun, error) {
	endpoint := c.regionURL("serverless-jobs", "v1alpha1") + "/job-runs/" + runID
	var run JobRun
	if err := c.doRequestJSON(ctx, "GET", endpoint, nil, &run); err != nil {
		return nil, AnnotateScalewayError(err, fmt.Sprintf("getting job run %q", runID))
	}
	return &run, nil
}

// ListJobRuns lists runs for a given job definition.
func (c *Client) ListJobRuns(ctx context.Context, definitionID string) ([]JobRun, error) {
	endpoint := c.regionURL("serverless-jobs", "v1alpha1") + "/job-runs"
	params := url.Values{
		"job_definition_id": {definitionID},
	}
	fullURL := endpoint + "?" + params.Encode()

	var resp listJobRunsResponse
	if err := c.doRequestJSON(ctx, "GET", fullURL, nil, &resp); err != nil {
		return nil, AnnotateScalewayError(err, fmt.Sprintf("listing job runs for %q", definitionID))
	}
	return resp.JobRuns, nil
}

// DeleteJobDefinition deletes a job definition.
func (c *Client) DeleteJobDefinition(ctx context.Context, definitionID string) error {
	endpoint := c.regionURL("serverless-jobs", "v1alpha1") + "/job-definitions/" + definitionID
	if err := c.doRequestJSON(ctx, "DELETE", endpoint, nil, nil); err != nil {
		return AnnotateScalewayError(err, fmt.Sprintf("deleting job definition %q", definitionID))
	}
	return nil
}
