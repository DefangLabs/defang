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
	StartupCommand  []string          `json:"startup_command,omitempty"`
	Args            []string          `json:"args,omitempty"`
	EnvironmentVars map[string]string `json:"environment_variables"`
	Region          string            `json:"region"`
	CreatedAt       string            `json:"created_at"`
}

// JobRun represents a single execution of a serverless job.
type JobRun struct {
	ID              string `json:"id"`
	JobDefinitionID string `json:"job_definition_id"`
	State           string `json:"state"`
	Reason          string `json:"reason,omitempty"`
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

type startJobDefinitionResponse struct {
	JobRuns []JobRun `json:"job_runs"`
}

type JobSecretRef struct {
	SecretManagerID      string
	SecretManagerVersion string
	EnvVarName           string
}

// CreateJobDefinition creates a new serverless job definition.
func (c *Client) CreateJobDefinition(ctx context.Context, name, image string, env map[string]string, resources JobResources) (*JobDefinition, error) {
	url := c.regionURL("serverless-jobs", "v1alpha2") + "/job-definitions"
	body := map[string]any{
		"name":                  name,
		"project_id":            c.ProjectID,
		"cpu_limit":             resources.CPULimit,
		"memory_limit":          resources.MemoryLimit,
		"image_uri":             image,
		"environment_variables": env,
	}
	var def JobDefinition
	if err := c.doRequestJSON(ctx, "POST", url, body, &def); err != nil {
		return nil, AnnotateScalewayError(err, fmt.Sprintf("creating job definition %q", name))
	}
	return &def, nil
}

// CreateJobSecrets attaches Secret Manager secrets to a Serverless Job definition.
func (c *Client) CreateJobSecrets(ctx context.Context, definitionID string, refs []JobSecretRef) error {
	if len(refs) == 0 {
		return nil
	}
	endpoint := c.regionURL("serverless-jobs", "v1alpha2") + "/secrets"
	secrets := make([]map[string]string, 0, len(refs))
	for _, ref := range refs {
		secrets = append(secrets, map[string]string{
			"secret_manager_id":      ref.SecretManagerID,
			"secret_manager_version": ref.SecretManagerVersion,
			"env_var_name":           ref.EnvVarName,
		})
	}
	body := map[string]any{
		"job_definition_id": definitionID,
		"secrets":           secrets,
	}
	if err := c.doRequestJSON(ctx, "POST", endpoint, body, nil); err != nil {
		return AnnotateScalewayError(err, fmt.Sprintf("creating secret references for job %q", definitionID))
	}
	return nil
}

// RunJob starts a new run of a job definition with optional command and environment overrides.
func (c *Client) RunJob(ctx context.Context, definitionID string, command []string, args []string, envOverrides map[string]string) (*JobRun, error) {
	endpoint := c.regionURL("serverless-jobs", "v1alpha2") + "/job-definitions/" + definitionID + "/start"
	body := map[string]any{}
	if len(command) > 0 {
		body["startup_command"] = command
	}
	if len(args) > 0 {
		body["args"] = args
	}
	if len(envOverrides) > 0 {
		body["environment_variables"] = envOverrides
	}
	var resp startJobDefinitionResponse
	if err := c.doRequestJSON(ctx, "POST", endpoint, body, &resp); err != nil {
		return nil, AnnotateScalewayError(err, fmt.Sprintf("running job %q", definitionID))
	}
	if len(resp.JobRuns) == 0 {
		return nil, fmt.Errorf("running job %q: no job runs returned", definitionID)
	}
	return &resp.JobRuns[0], nil
}

// GetJobRun retrieves the status of a specific job run.
func (c *Client) GetJobRun(ctx context.Context, runID string) (*JobRun, error) {
	endpoint := c.regionURL("serverless-jobs", "v1alpha2") + "/job-runs/" + runID
	var run JobRun
	if err := c.doRequestJSON(ctx, "GET", endpoint, nil, &run); err != nil {
		return nil, AnnotateScalewayError(err, fmt.Sprintf("getting job run %q", runID))
	}
	return &run, nil
}

// ListJobRuns lists runs for a given job definition.
func (c *Client) ListJobRuns(ctx context.Context, definitionID string) ([]JobRun, error) {
	endpoint := c.regionURL("serverless-jobs", "v1alpha2") + "/job-runs"
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

type listJobDefinitionsResponse struct {
	JobDefinitions []JobDefinition `json:"job_definitions"`
	TotalCount     int             `json:"total_count"`
}

// ListJobDefinitions lists job definitions in the project, optionally filtered by name.
func (c *Client) ListJobDefinitions(ctx context.Context, name string) ([]JobDefinition, error) {
	endpoint := c.regionURL("serverless-jobs", "v1alpha2") + "/job-definitions"
	params := url.Values{
		"project_id": {c.ProjectID},
	}
	if name != "" {
		params.Set("name", name)
	}
	fullURL := endpoint + "?" + params.Encode()

	var resp listJobDefinitionsResponse
	if err := c.doRequestJSON(ctx, "GET", fullURL, nil, &resp); err != nil {
		return nil, AnnotateScalewayError(err, "listing job definitions")
	}
	return resp.JobDefinitions, nil
}

// DeleteJobDefinition deletes a job definition.
func (c *Client) DeleteJobDefinition(ctx context.Context, definitionID string) error {
	endpoint := c.regionURL("serverless-jobs", "v1alpha2") + "/job-definitions/" + definitionID
	if err := c.doRequestJSON(ctx, "DELETE", endpoint, nil, nil); err != nil {
		return AnnotateScalewayError(err, fmt.Sprintf("deleting job definition %q", definitionID))
	}
	return nil
}
