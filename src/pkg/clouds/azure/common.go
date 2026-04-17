package azure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage/v2"
)

// cliTimeout overrides the default 10s timeout for CLI-based credentials.
// The Azure CLI can be slow to start, especially when installed via Nix.
const cliTimeout = 30 * time.Second

type Azure struct {
	Location       Location
	SubscriptionID string
}

// tokenCredentialWithTimeout wraps an azcore.TokenCredential to ensure
// GetToken has a minimum deadline, overriding the SDK's default 10s CLI timeout.
type tokenCredentialWithTimeout struct {
	cred    azcore.TokenCredential
	timeout time.Duration
}

func (t *tokenCredentialWithTimeout) GetToken(ctx context.Context, opts policy.TokenRequestOptions) (azcore.AccessToken, error) {
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, t.timeout)
		defer cancel()
	}
	return t.cred.GetToken(ctx, opts)
}

// NewCredsFunc builds a TokenCredential for ARM calls. Tests can override this
// to inject a fake credential; the default implementation is DefaultAzureCredential.
var NewCredsFunc = func(a Azure) (azcore.TokenCredential, error) {
	if len(a.SubscriptionID) == 0 {
		return nil, errors.New("environment variable AZURE_SUBSCRIPTION_ID is not set")
	}

	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create default Azure credentials: %w", err)
	}

	return &tokenCredentialWithTimeout{cred: cred, timeout: cliTimeout}, nil
}

func (a Azure) NewCreds() (azcore.TokenCredential, error) {
	return NewCredsFunc(a)
}

// ManagementEndpoint is the base URL for Azure Resource Manager REST calls.
// It is exposed as a variable so tests can swap in an httptest.Server URL.
var ManagementEndpoint = "https://management.azure.com"

// ArmToken returns a Bearer token scoped to the Azure management endpoint,
// suitable for direct REST API calls that the ARM SDK does not expose.
func (a Azure) ArmToken(ctx context.Context) (string, error) {
	cred, err := a.NewCreds()
	if err != nil {
		return "", err
	}
	tok, err := cred.GetToken(ctx, policy.TokenRequestOptions{
		Scopes: []string{"https://management.azure.com/.default"},
	})
	if err != nil {
		return "", err
	}
	return tok.Token, nil
}

// FetchLogStreamAuthToken POSTs to the `getAuthToken` action on an ACA resource
// (container app or job) and returns the short-lived token that the resource's
// log-stream endpoint accepts. resourcePath is the segment after
// "providers/", e.g. "Microsoft.App/containerApps/{name}" or
// "Microsoft.App/jobs/{name}".
func (a Azure) FetchLogStreamAuthToken(ctx context.Context, resourceGroup, resourcePath, apiVersion string) (string, error) {
	armTok, err := a.ArmToken(ctx)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf(
		"%s/subscriptions/%s/resourceGroups/%s/providers/%s/getAuthToken?api-version=%s",
		ManagementEndpoint, a.SubscriptionID, resourceGroup, resourcePath, apiVersion,
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, http.NoBody)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+armTok)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("getAuthToken: HTTP %s", resp.Status)
	}

	var result struct {
		Properties struct {
			Token string `json:"token"`
		} `json:"properties"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("getAuthToken: decode response: %w", err)
	}
	return result.Properties.Token, nil
}

func (a Azure) NewStorageAccountsClient() (*armstorage.AccountsClient, error) {
	cred, err := a.NewCreds()
	if err != nil {
		return nil, err
	}

	clientFactory, err := armstorage.NewClientFactory(a.SubscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create storage client: %w", err)
	}

	return clientFactory.NewAccountsClient(), nil
}

func (a Azure) NewBlobContainersClient() (*armstorage.BlobContainersClient, error) {
	cred, err := a.NewCreds()
	if err != nil {
		return nil, err
	}

	clientFactory, err := armstorage.NewClientFactory(a.SubscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create storage client: %w", err)
	}

	return clientFactory.NewBlobContainersClient(), nil
}

// func (a Azure) NewRoleAssignmentsClient() (*armauthorization.RoleAssignmentsClient, error) {
// 	cred, err := a.NewCreds()
// 	if err != nil {
// 		return nil, err
// 	}

// 	clientFactory, err := armauthorization.NewRoleAssignmentsClient(a.SubscriptionID, cred, nil)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to create role assignments client: %w", err)
// 	}

// 	return clientFactory, nil
// }
