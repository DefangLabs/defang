package azure

import (
	"context"
	"errors"
	"fmt"
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

func (a Azure) NewCreds() (azcore.TokenCredential, error) {
	if len(a.SubscriptionID) == 0 {
		return nil, errors.New("environment variable AZURE_SUBSCRIPTION_ID is not set")
	}

	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create default Azure credentials: %w", err)
	}

	return &tokenCredentialWithTimeout{cred: cred, timeout: cliTimeout}, nil
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
