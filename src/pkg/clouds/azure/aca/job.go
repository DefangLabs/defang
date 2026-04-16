package aca

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"net/http"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	armappcontainersv3 "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appcontainers/armappcontainers/v3"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization/v2"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/operationalinsights/armoperationalinsights"
	cloudazure "github.com/DefangLabs/defang/src/pkg/clouds/azure"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/google/uuid"
)

const (
	cdJobName          = "defang-cd"
	cdEnvironmentName  = "defang-cd"
	cdLogWorkspaceName = "defang-cd"
	jobLogPollInterval = 3 * time.Second
)

// JobRequest contains parameters for starting a Container Apps Job execution.
type JobRequest struct {
	Image   string
	Command []string
	// Envs are plain-text environment variables.
	Envs map[string]string
	// SecretEnvs are environment variables that should be stored as secrets (not shown in plain text).
	SecretEnvs map[string]string
	// Timeout is the maximum execution duration.
	Timeout time.Duration
}

// JobStatus represents the status of a Container Apps Job execution.
type JobStatus struct {
	ExecutionName string
	Status        armappcontainersv3.JobExecutionRunningState
	ErrorMessage  string
}

// IsTerminal returns true if the execution has reached a final state.
func (s *JobStatus) IsTerminal() bool {
	switch s.Status {
	case armappcontainersv3.JobExecutionRunningStateSucceeded,
		armappcontainersv3.JobExecutionRunningStateFailed,
		armappcontainersv3.JobExecutionRunningStateStopped,
		armappcontainersv3.JobExecutionRunningStateDegraded:
		return true
	}
	return false
}

// IsSuccess returns true if the execution completed successfully.
func (s *JobStatus) IsSuccess() bool {
	return s.Status == armappcontainersv3.JobExecutionRunningStateSucceeded
}

// Job manages Container Apps Jobs and the environment they run in.
// It owns the CD job lifecycle: creating the environment, setting up the job,
// running executions, streaming logs, and assigning the job's managed identity roles.
type Job struct {
	cloudazure.Azure
	ResourceGroup     string
	EnvironmentID     string
	SystemPrincipalID string
	cdJobImage        string
	identitySetUp     bool
}

func (j *Job) newManagedEnvironmentsClient() (*armappcontainersv3.ManagedEnvironmentsClient, error) {
	cred, err := j.NewCreds()
	if err != nil {
		return nil, err
	}
	client, err := armappcontainersv3.NewManagedEnvironmentsClient(j.SubscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create managed environments client: %w", err)
	}
	return client, nil
}

func (j *Job) newJobsClient() (*armappcontainersv3.JobsClient, error) {
	cred, err := j.NewCreds()
	if err != nil {
		return nil, err
	}
	client, err := armappcontainersv3.NewJobsClient(j.SubscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create jobs client: %w", err)
	}
	return client, nil
}

func (j *Job) newJobsExecutionsClient() (*armappcontainersv3.JobsExecutionsClient, error) {
	cred, err := j.NewCreds()
	if err != nil {
		return nil, err
	}
	client, err := armappcontainersv3.NewJobsExecutionsClient(j.SubscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create jobs executions client: %w", err)
	}
	return client, nil
}

// setUpLogWorkspace creates (or retrieves) a Log Analytics workspace and returns its
// customer ID (workspace GUID) and primary shared key, which are needed to configure
// ACA environment log streaming.
func (j *Job) setUpLogWorkspace(ctx context.Context) (customerID, sharedKey string, err error) {
	cred, err := j.NewCreds()
	if err != nil {
		return "", "", err
	}
	wsClient, err := armoperationalinsights.NewWorkspacesClient(j.SubscriptionID, cred, nil)
	if err != nil {
		return "", "", fmt.Errorf("failed to create log analytics workspaces client: %w", err)
	}

	// Create or update the workspace (idempotent).
	term.Debugf("setUpLogWorkspace: creating/updating workspace %q in %q", cdLogWorkspaceName, j.ResourceGroup)
	wsPoller, err := wsClient.BeginCreateOrUpdate(ctx, j.ResourceGroup, cdLogWorkspaceName, armoperationalinsights.Workspace{
		Location: j.Location.Ptr(),
		Properties: &armoperationalinsights.WorkspaceProperties{
			SKU: &armoperationalinsights.WorkspaceSKU{
				Name: to.Ptr(armoperationalinsights.WorkspaceSKUNameEnumPerGB2018),
			},
			RetentionInDays: to.Ptr(int32(30)),
		},
	}, nil)
	if err != nil {
		return "", "", fmt.Errorf("failed to create log analytics workspace: %w", err)
	}
	wsResult, err := wsPoller.PollUntilDone(ctx, nil)
	if err != nil {
		return "", "", fmt.Errorf("failed to poll workspace creation: %w", err)
	}
	if wsResult.Properties == nil || wsResult.Properties.CustomerID == nil {
		return "", "", errors.New("log analytics workspace did not return a customer ID")
	}
	customerID = *wsResult.Properties.CustomerID

	// Fetch the shared key (not available on the workspace resource itself).
	keysClient, err := armoperationalinsights.NewSharedKeysClient(j.SubscriptionID, cred, nil)
	if err != nil {
		return "", "", fmt.Errorf("failed to create shared keys client: %w", err)
	}
	keysResp, err := keysClient.GetSharedKeys(ctx, j.ResourceGroup, cdLogWorkspaceName, nil)
	if err != nil {
		return "", "", fmt.Errorf("failed to get workspace shared keys: %w", err)
	}
	if keysResp.PrimarySharedKey == nil {
		return "", "", errors.New("log analytics workspace returned no primary shared key")
	}
	return customerID, *keysResp.PrimarySharedKey, nil
}

// SetUpEnvironment creates (or retrieves) the Container Apps Environment that hosts the CD job.
// It also creates a Log Analytics workspace and configures the environment to stream logs
// there so they're visible in the Azure portal and via Log Analytics queries.
// The environment resource ID is stored in j.EnvironmentID.
func (j *Job) SetUpEnvironment(ctx context.Context) error {
	if j.EnvironmentID != "" {
		term.Debugf("SetUpEnvironment: already set (%s)", j.EnvironmentID)
		return nil
	}

	// Set up Log Analytics workspace first so we can wire it into the environment.
	customerID, sharedKey, err := j.setUpLogWorkspace(ctx)
	if err != nil {
		return err
	}

	envClient, err := j.newManagedEnvironmentsClient()
	if err != nil {
		return err
	}

	appLogsConfig := &armappcontainersv3.AppLogsConfiguration{
		Destination: to.Ptr("log-analytics"),
		LogAnalyticsConfiguration: &armappcontainersv3.LogAnalyticsConfiguration{
			CustomerID: to.Ptr(customerID),
			SharedKey:  to.Ptr(sharedKey),
		},
	}

	term.Debugf("SetUpEnvironment: checking if %q exists in %q", cdEnvironmentName, j.ResourceGroup)
	if resp, err := envClient.Get(ctx, j.ResourceGroup, cdEnvironmentName, nil); err == nil {
		// Environment exists. Ensure its AppLogsConfiguration points to our workspace
		// (idempotent update — safe to run on every call).
		term.Debugf("SetUpEnvironment: updating existing environment %s to use Log Analytics", *resp.ID)
		updatePoller, err := envClient.BeginCreateOrUpdate(ctx, j.ResourceGroup, cdEnvironmentName, armappcontainersv3.ManagedEnvironment{
			Location: j.Location.Ptr(),
			Properties: &armappcontainersv3.ManagedEnvironmentProperties{
				ZoneRedundant:        to.Ptr(false),
				AppLogsConfiguration: appLogsConfig,
			},
		}, nil)
		if err != nil {
			return fmt.Errorf("failed to update container apps environment: %w", err)
		}
		result, err := updatePoller.PollUntilDone(ctx, nil)
		if err != nil {
			return fmt.Errorf("failed to poll environment update: %w", err)
		}
		j.EnvironmentID = *result.ID
		return nil
	}

	term.Infof("Creating Container Apps environment %q in %q", cdEnvironmentName, j.ResourceGroup)
	poller, err := envClient.BeginCreateOrUpdate(ctx, j.ResourceGroup, cdEnvironmentName, armappcontainersv3.ManagedEnvironment{
		Location: j.Location.Ptr(),
		Properties: &armappcontainersv3.ManagedEnvironmentProperties{
			ZoneRedundant:        to.Ptr(false),
			AppLogsConfiguration: appLogsConfig,
		},
	}, nil)
	if err != nil {
		return fmt.Errorf("failed to create container apps environment: %w", err)
	}
	result, err := poller.PollUntilDone(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to poll environment creation: %w", err)
	}
	j.EnvironmentID = *result.ID
	term.Infof("Created Container Apps environment %s", j.EnvironmentID)
	return nil
}

// Well-known Azure built-in role definition IDs.
const (
	storageBlobDataContributorRoleID = "ba92f5b4-2d11-453d-a403-e96b0029c9fe" // nolint:gosec
	contributorRoleID                = "b24988ac-6180-42a0-ab88-20f7382dd24c" // nolint:gosec
	userAccessAdministratorRoleID    = "18d7d88d-d35e-4fb5-a5c3-7773c20a72d9" // nolint:gosec
	keyVaultSecretsUserRoleID        = "4633458b-17de-408a-b874-0445c86b69e6" // nolint:gosec
)

// assignRole assigns a built-in role to the given principal at the given scope.
// It silently ignores RoleAssignmentExists errors (idempotent).
func assignRole(ctx context.Context, raClient *armauthorization.RoleAssignmentsClient, subscriptionID, scope, roleDefID, principalID string) error {
	fullRoleDefID := fmt.Sprintf("/subscriptions/%s/providers/Microsoft.Authorization/roleDefinitions/%s", subscriptionID, roleDefID)
	_, err := raClient.Create(ctx, scope, uuid.NewString(), armauthorization.RoleAssignmentCreateParameters{
		Properties: &armauthorization.RoleAssignmentProperties{
			PrincipalID:      to.Ptr(principalID),
			RoleDefinitionID: to.Ptr(fullRoleDefID),
			PrincipalType:    to.Ptr(armauthorization.PrincipalTypeServicePrincipal),
		},
	}, nil)
	if err != nil {
		var respErr *azcore.ResponseError
		if !errors.As(err, &respErr) || respErr.ErrorCode != "RoleAssignmentExists" {
			return err
		}
	}
	return nil
}

// SetUpManagedIdentity assigns the necessary roles to the CD job's system-assigned managed
// identity so it can provision Azure resources and access Pulumi state in storageAccount.
// SetUpJob must be called before this to populate SystemPrincipalID.
func (j *Job) SetUpManagedIdentity(ctx context.Context, storageAccount string) error {
	if j.identitySetUp {
		return nil
	}
	if j.SystemPrincipalID == "" {
		return errors.New("CD job system-assigned identity principal ID is not set; ensure SetUpJob was called first")
	}

	cred, err := j.NewCreds()
	if err != nil {
		return err
	}
	raClient, err := armauthorization.NewRoleAssignmentsClient(j.SubscriptionID, cred, nil)
	if err != nil {
		return fmt.Errorf("failed to create role assignments client: %w", err)
	}

	// Contributor + User Access Administrator on the subscription so Pulumi can provision any
	// Azure resource and create role assignments (e.g. ACR pull role for Container Apps).
	subscriptionScope := "/subscriptions/" + j.SubscriptionID
	if err := assignRole(ctx, raClient, j.SubscriptionID, subscriptionScope, contributorRoleID, j.SystemPrincipalID); err != nil {
		return fmt.Errorf("failed to assign Contributor role: %w", err)
	}
	if err := assignRole(ctx, raClient, j.SubscriptionID, subscriptionScope, userAccessAdministratorRoleID, j.SystemPrincipalID); err != nil {
		return fmt.Errorf("failed to assign User Access Administrator role: %w", err)
	}
	// Key Vault Secrets User on the subscription so the CD container can read project
	// secrets from the Key Vault (used both by the CD itself and by Pulumi).
	if err := assignRole(ctx, raClient, j.SubscriptionID, subscriptionScope, keyVaultSecretsUserRoleID, j.SystemPrincipalID); err != nil {
		return fmt.Errorf("failed to assign Key Vault Secrets User role: %w", err)
	}

	// Storage Blob Data Contributor on the storage account for Pulumi state and payload access.
	storageScope := fmt.Sprintf(
		"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Storage/storageAccounts/%s",
		j.SubscriptionID, j.ResourceGroup, storageAccount,
	)
	if err := assignRole(ctx, raClient, j.SubscriptionID, storageScope, storageBlobDataContributorRoleID, j.SystemPrincipalID); err != nil {
		return fmt.Errorf("failed to assign Storage Blob Data Contributor role: %w", err)
	}

	j.identitySetUp = true
	return nil
}

// SetUpJob creates (or updates) the Container Apps Job used to run the CD container.
// Environment variables are baked into the job template so they're available to every
// execution (the execution-time override for env vars is unreliable — the job template is
// the authoritative source).
// The CD image is pulled anonymously; the image's registry must allow anonymous pull.
// It enables a system-assigned managed identity on the job and stores the principal ID
// in j.SystemPrincipalID for subsequent role assignments.
// SetUpEnvironment must be called first.
func (j *Job) SetUpJob(ctx context.Context, image string, envMap map[string]string) error {
	if j.EnvironmentID == "" {
		return errors.New("environment ID is not set; ensure SetUpEnvironment was called first")
	}

	term.Debugf("SetUpJob: creating/updating job %q with image %q (%d env vars)", cdJobName, image, len(envMap))
	jobsClient, err := j.newJobsClient()
	if err != nil {
		return err
	}

	var envVars []*armappcontainersv3.EnvironmentVar
	for k, v := range envMap {
		envVars = append(envVars, &armappcontainersv3.EnvironmentVar{
			Name:  to.Ptr(k),
			Value: to.Ptr(v),
		})
	}

	timeout := int32((30 * time.Minute).Seconds())
	const tmpVolumeName = "tmp"
	poller, err := jobsClient.BeginCreateOrUpdate(ctx, j.ResourceGroup, cdJobName, armappcontainersv3.Job{
		Location: j.Location.Ptr(),
		Identity: &armappcontainersv3.ManagedServiceIdentity{
			Type: to.Ptr(armappcontainersv3.ManagedServiceIdentityTypeSystemAssigned),
		},
		Properties: &armappcontainersv3.JobProperties{
			EnvironmentID: to.Ptr(j.EnvironmentID),
			Configuration: &armappcontainersv3.JobConfiguration{
				TriggerType:       to.Ptr(armappcontainersv3.TriggerTypeManual),
				ReplicaTimeout:    to.Ptr(timeout),
				ReplicaRetryLimit: to.Ptr(int32(0)),
			},
			Template: &armappcontainersv3.JobTemplate{
				Volumes: []*armappcontainersv3.Volume{
					{
						Name:        to.Ptr(tmpVolumeName),
						StorageType: to.Ptr(armappcontainersv3.StorageTypeEmptyDir),
					},
				},
				Containers: []*armappcontainersv3.Container{
					{
						Name:  to.Ptr(cdJobName),
						Image: to.Ptr(image),
						Env:   envVars,
						VolumeMounts: []*armappcontainersv3.VolumeMount{
							{
								VolumeName: to.Ptr(tmpVolumeName),
								MountPath:  to.Ptr("/tmp"),
							},
						},
					},
				},
			},
		},
	}, nil)
	if err != nil {
		return fmt.Errorf("failed to create/update CD job: %w", err)
	}

	result, err := poller.PollUntilDone(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to poll CD job creation: %w", err)
	}

	if result.Identity != nil && result.Identity.PrincipalID != nil {
		j.SystemPrincipalID = *result.Identity.PrincipalID
	}
	j.cdJobImage = image
	return nil
}

// StartJobExecution starts a new execution of the CD job with the given image, command,
// and environment variables. Returns the execution name.
func (j *Job) StartJobExecution(ctx context.Context, req JobRequest) (string, error) {
	jobsClient, err := j.newJobsClient()
	if err != nil {
		return "", err
	}

	// Build environment variable list. Secrets are stored on the job and referenced by name.
	var envVars []*armappcontainersv3.EnvironmentVar
	var secrets []*armappcontainersv3.Secret
	for k, v := range req.Envs {
		envVars = append(envVars, &armappcontainersv3.EnvironmentVar{
			Name:  to.Ptr(k),
			Value: to.Ptr(v),
		})
	}
	for k, v := range req.SecretEnvs {
		secretName := strings.ToLower(strings.ReplaceAll(k, "_", "-"))
		secrets = append(secrets, &armappcontainersv3.Secret{
			Name:  to.Ptr(secretName),
			Value: to.Ptr(v),
		})
		envVars = append(envVars, &armappcontainersv3.EnvironmentVar{
			Name:      to.Ptr(k),
			SecretRef: to.Ptr(secretName),
		})
	}

	// Update job secrets if any were added.
	if len(secrets) > 0 {
		secretsPoller, err := jobsClient.BeginUpdate(ctx, j.ResourceGroup, cdJobName, armappcontainersv3.JobPatchProperties{
			Properties: &armappcontainersv3.JobPatchPropertiesProperties{
				Configuration: &armappcontainersv3.JobConfiguration{
					TriggerType:    to.Ptr(armappcontainersv3.TriggerTypeManual),
					ReplicaTimeout: to.Ptr(int32((30 * time.Minute).Seconds())),
					Secrets:        secrets,
				},
			},
		}, nil)
		if err != nil {
			return "", fmt.Errorf("failed to update job secrets: %w", err)
		}
		if _, err := secretsPoller.PollUntilDone(ctx, nil); err != nil {
			return "", fmt.Errorf("failed to poll job secrets update: %w", err)
		}
	}

	// Build the command args list.
	var args []*string
	for _, a := range req.Command[1:] {
		args = append(args, to.Ptr(a))
	}
	var cmd []*string
	if len(req.Command) > 0 {
		cmd = []*string{to.Ptr(req.Command[0])}
	}

	execContainer := &armappcontainersv3.JobExecutionContainer{
		Name:  to.Ptr(cdJobName),
		Image: to.Ptr(req.Image),
		Env:   envVars,
	}
	if len(cmd) > 0 {
		execContainer.Command = cmd
		execContainer.Args = args
	}

	poller, err := jobsClient.BeginStart(ctx, j.ResourceGroup, cdJobName, &armappcontainersv3.JobsClientBeginStartOptions{
		Template: &armappcontainersv3.JobExecutionTemplate{
			Containers: []*armappcontainersv3.JobExecutionContainer{execContainer},
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to start job execution: %w", err)
	}

	result, err := poller.PollUntilDone(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("failed to poll job start: %w", err)
	}

	if result.Name == nil {
		return "", errors.New("job execution started but returned no name")
	}
	return *result.Name, nil
}

// GetJobExecutionStatus returns the current status of a job execution by listing executions
// and finding the one with the given name.
func (j *Job) GetJobExecutionStatus(ctx context.Context, executionName string) (*JobStatus, error) {
	execClient, err := j.newJobsExecutionsClient()
	if err != nil {
		return nil, err
	}

	pager := execClient.NewListPager(j.ResourceGroup, cdJobName, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list job executions: %w", err)
		}
		for _, exec := range page.Value {
			if exec.Name != nil && *exec.Name == executionName {
				status := &JobStatus{ExecutionName: executionName}
				if exec.Properties != nil && exec.Properties.Status != nil {
					status.Status = *exec.Properties.Status
				}
				return status, nil
			}
		}
	}
	return nil, fmt.Errorf("execution %q not found", executionName)
}

// TailJobLogs returns an iter that yields a single terminal error if the CD job
// failed, and nothing otherwise. Container output is NOT streamed here — the
// ByocAzure provider drains and prints CD logs synchronously inside its
// GetDeploymentStatus method (which the CLI polls via WaitForCdTaskExit). Doing
// the printing there guarantees the program stays alive long enough for Azure
// Log Analytics to ingest+index the container output, which can lag by several
// minutes behind real time and is unworkable for a streaming iter.
func (j *Job) TailJobLogs(ctx context.Context, executionName string) (iter.Seq2[string, error], error) {
	return func(yield func(string, error) bool) {
		for {
			status, err := j.GetJobExecutionStatus(ctx, executionName)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				yield("", fmt.Errorf("failed to get execution status: %w", err))
				return
			}
			if status.IsTerminal() {
				if !status.IsSuccess() {
					msg := string(status.Status)
					if status.ErrorMessage != "" {
						msg += ": " + status.ErrorMessage
					}
					yield("", fmt.Errorf("CD job %s: %s", executionName, msg))
				}
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(jobLogPollInterval):
			}
		}
	}, nil
}

// ReadJobLogs returns all log output captured for a job execution from Log Analytics.
// Subject to a short ingestion delay (seconds to a couple of minutes on cold workspaces).
func (j *Job) ReadJobLogs(ctx context.Context, executionName string) (string, error) {
	return j.fetchLogsFromWorkspace(ctx, executionName)
}

// getLogAnalyticsToken returns a Bearer token for the Log Analytics query API.
func (j *Job) getLogAnalyticsToken(ctx context.Context) (string, error) {
	cred, err := j.NewCreds()
	if err != nil {
		return "", err
	}
	tok, err := cred.GetToken(ctx, policy.TokenRequestOptions{
		Scopes: []string{"https://api.loganalytics.io/.default"},
	})
	if err != nil {
		return "", err
	}
	return tok.Token, nil
}

// getLogWorkspaceCustomerID returns the customer ID (GUID) of the CD Log Analytics
// workspace. This is what the Log Analytics query API addresses workspaces by.
func (j *Job) getLogWorkspaceCustomerID(ctx context.Context) (string, error) {
	cred, err := j.NewCreds()
	if err != nil {
		return "", err
	}
	wsClient, err := armoperationalinsights.NewWorkspacesClient(j.SubscriptionID, cred, nil)
	if err != nil {
		return "", fmt.Errorf("creating log analytics workspaces client: %w", err)
	}
	resp, err := wsClient.Get(ctx, j.ResourceGroup, cdLogWorkspaceName, nil)
	if err != nil {
		return "", fmt.Errorf("getting log analytics workspace: %w", err)
	}
	if resp.Properties == nil || resp.Properties.CustomerID == nil {
		return "", errors.New("log analytics workspace has no customer ID")
	}
	return *resp.Properties.CustomerID, nil
}

// fetchLogsFromWorkspace queries Log Analytics for all console log entries belonging to
// the given job execution, ordered by time. Returns empty string when the workspace has
// no rows yet (first-time workspaces can take 2–5 minutes to ingest data).
func (j *Job) fetchLogsFromWorkspace(ctx context.Context, executionName string) (string, error) {
	workspaceID, err := j.getLogWorkspaceCustomerID(ctx)
	if err != nil {
		return "", err
	}
	token, err := j.getLogAnalyticsToken(ctx)
	if err != nil {
		return "", err
	}

	// Filter by pod name (ContainerGroupName_s), which has the form
	// "{executionName}-{randomsuffix}" — ContainerJobName_s is always just the job name
	// ("defang-cd") so it can't disambiguate executions. Execution names are Azure-generated
	// alphanumeric + hyphens, so no quoting hazard inlining them into the query.
	query := fmt.Sprintf(
		`ContainerAppConsoleLogs_CL `+
			`| where ContainerName_s == "%s" and ContainerGroupName_s startswith "%s-" `+
			`| order by TimeGenerated asc `+
			`| project TimeGenerated, Log_s`,
		cdJobName, executionName,
	)
	body, err := json.Marshal(map[string]string{"query": query})
	if err != nil {
		return "", err
	}

	url := "https://api.loganalytics.io/v1/workspaces/" + workspaceID + "/query"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("log analytics query: HTTP %s", resp.Status)
	}

	var result struct {
		Tables []struct {
			Rows [][]any `json:"rows"`
		} `json:"tables"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode log analytics response: %w", err)
	}

	var sb strings.Builder
	if len(result.Tables) > 0 {
		for _, row := range result.Tables[0].Rows {
			if len(row) < 2 {
				continue
			}
			ts, _ := row[0].(string)
			line, _ := row[1].(string)
			sb.WriteString(ts)
			sb.WriteByte(' ')
			sb.WriteString(line)
			if !strings.HasSuffix(line, "\n") {
				sb.WriteByte('\n')
			}
		}
	}
	return sb.String(), nil
}
