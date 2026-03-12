package acr

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerregistry/armcontainerregistry/v2"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources/v2"
	"github.com/DefangLabs/defang/src/pkg/clouds/azure"
)

type ACR struct {
	azure.Azure
	resourceGroupName string
	RegistryName      string
}

func New(resourceGroupPrefix string, location azure.Location) *ACR {
	if location == "" {
		location = azure.Location(os.Getenv("AZURE_LOCATION"))
	}
	return &ACR{
		Azure: azure.Azure{
			Location:       location,
			SubscriptionID: os.Getenv("AZURE_SUBSCRIPTION_ID"),
		},
		resourceGroupName: resourceGroupPrefix + location.String(),
	}
}

func (a *ACR) newRegistriesClient() (*armcontainerregistry.RegistriesClient, error) {
	cred, err := a.NewCreds()
	if err != nil {
		return nil, err
	}
	client, err := armcontainerregistry.NewRegistriesClient(a.SubscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create registries client: %w", err)
	}
	return client, nil
}

func (a *ACR) newResourceGroupClient() (*armresources.ResourceGroupsClient, error) {
	cred, err := a.NewCreds()
	if err != nil {
		return nil, err
	}
	client, err := armresources.NewResourceGroupsClient(a.SubscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource group client: %w", err)
	}
	return client, nil
}

func (a *ACR) newRunsClient() (*armcontainerregistry.RunsClient, error) {
	cred, err := a.NewCreds()
	if err != nil {
		return nil, err
	}
	client, err := armcontainerregistry.NewRunsClient(a.SubscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create runs client: %w", err)
	}
	return client, nil
}

// SetUpRegistry ensures the resource group exists and creates a Basic-tier ACR.
func (a *ACR) SetUpRegistry(ctx context.Context, registryName string) error {
	rgClient, err := a.newResourceGroupClient()
	if err != nil {
		return err
	}
	_, err = rgClient.CreateOrUpdate(ctx, a.resourceGroupName, armresources.ResourceGroup{
		Location: a.Location.Ptr(),
	}, nil)
	if err != nil {
		return fmt.Errorf("failed to create resource group: %w", err)
	}

	client, err := a.newRegistriesClient()
	if err != nil {
		return err
	}

	a.RegistryName = registryName

	poller, err := client.BeginCreate(ctx, a.resourceGroupName, registryName, armcontainerregistry.Registry{
		Location: a.Location.Ptr(),
		SKU: &armcontainerregistry.SKU{
			Name: to.Ptr(armcontainerregistry.SKUNameBasic),
		},
	}, nil)
	if err != nil {
		return fmt.Errorf("failed to create container registry: %w", err)
	}

	_, err = poller.PollUntilDone(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to poll registry creation: %w", err)
	}

	return nil
}

// GetBuildSourceUploadURL returns a URL where the build context can be uploaded.
func (a *ACR) GetBuildSourceUploadURL(ctx context.Context) (uploadURL, relativePath string, err error) {
	client, err := a.newRegistriesClient()
	if err != nil {
		return "", "", err
	}

	resp, err := client.GetBuildSourceUploadURL(ctx, a.resourceGroupName, a.RegistryName, nil)
	if err != nil {
		return "", "", fmt.Errorf("failed to get source upload URL: %w", err)
	}

	return *resp.UploadURL, *resp.RelativePath, nil
}

// RunTask schedules an ACR task that runs a container image and returns the run ID.
func (a *ACR) RunTask(ctx context.Context, req TaskRequest) (string, error) {
	client, err := a.newRegistriesClient()
	if err != nil {
		return "", err
	}

	// Build the YAML task definition with a single cmd step
	var yaml strings.Builder
	yaml.WriteString("steps:\n")
	yaml.WriteString("  - cmd: " + req.Image)
	if len(req.Command) > 0 {
		// Arguments after the image are passed as the command
		yaml.WriteString(" " + strings.Join(req.Command, " "))
	}
	yaml.WriteString("\n")

	runRequest := &armcontainerregistry.EncodedTaskRunRequest{
		Type:               to.Ptr("EncodedTaskRunRequest"),
		EncodedTaskContent: to.Ptr(base64.StdEncoding.EncodeToString([]byte(yaml.String()))),
		Platform: &armcontainerregistry.PlatformProperties{
			OS:           to.Ptr(armcontainerregistry.OSLinux),
			Architecture: to.Ptr(armcontainerregistry.ArchitectureAmd64),
		},
		IsArchiveEnabled: to.Ptr(false),
		Timeout:          to.Ptr(int32(req.Timeout.Seconds())),
	}

	if req.SourceLocation != "" {
		runRequest.SourceLocation = to.Ptr(req.SourceLocation)
	}

	// Pass env vars and secrets as task values
	for k, v := range req.Envs {
		runRequest.Values = append(runRequest.Values, &armcontainerregistry.SetValue{
			Name:     to.Ptr(k),
			Value:    to.Ptr(v),
			IsSecret: to.Ptr(false),
		})
	}
	for k, v := range req.SecretEnvs {
		runRequest.Values = append(runRequest.Values, &armcontainerregistry.SetValue{
			Name:     to.Ptr(k),
			Value:    to.Ptr(v),
			IsSecret: to.Ptr(true),
		})
	}

	poller, err := client.BeginScheduleRun(ctx, a.resourceGroupName, a.RegistryName, runRequest, nil)
	if err != nil {
		return "", fmt.Errorf("failed to schedule task: %w", err)
	}

	result, err := poller.PollUntilDone(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("failed to poll scheduled run: %w", err)
	}

	if result.Run.Properties == nil || result.Run.Properties.RunID == nil {
		return "", errors.New("scheduled run has no run ID")
	}

	return *result.Run.Properties.RunID, nil
}

// GetRunStatus returns the current status of a run.
func (a *ACR) GetRunStatus(ctx context.Context, runID string) (*RunStatus, error) {
	client, err := a.newRunsClient()
	if err != nil {
		return nil, err
	}

	resp, err := client.Get(ctx, a.resourceGroupName, a.RegistryName, runID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get run: %w", err)
	}

	props := resp.Run.Properties
	if props == nil {
		return nil, fmt.Errorf("run %s has no properties", runID)
	}

	status := &RunStatus{
		RunID:  runID,
		Status: *props.Status,
	}
	if props.RunErrorMessage != nil {
		status.ErrorMessage = *props.RunErrorMessage
	}
	return status, nil
}

// GetRunLogURL returns a SAS URL to download the run's logs.
func (a *ACR) GetRunLogURL(ctx context.Context, runID string) (string, error) {
	client, err := a.newRunsClient()
	if err != nil {
		return "", err
	}

	resp, err := client.GetLogSasURL(ctx, a.resourceGroupName, a.RegistryName, runID, nil)
	if err != nil {
		return "", fmt.Errorf("failed to get log URL: %w", err)
	}

	if resp.LogLink == nil {
		return "", fmt.Errorf("no log URL for run %s", runID)
	}
	return *resp.LogLink, nil
}

// CancelRun cancels a running ACR task.
func (a *ACR) CancelRun(ctx context.Context, runID string) error {
	client, err := a.newRunsClient()
	if err != nil {
		return err
	}

	poller, err := client.BeginCancel(ctx, a.resourceGroupName, a.RegistryName, runID, nil)
	if err != nil {
		return fmt.Errorf("failed to cancel run: %w", err)
	}

	_, err = poller.PollUntilDone(ctx, nil)
	return err
}

// LoginServer returns the ACR login server URL (e.g. "myregistry.azurecr.io").
func (a *ACR) LoginServer() string {
	return a.RegistryName + ".azurecr.io"
}
