package azure

import (
	"errors"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage"
)

type Azure struct {
	Location       Location
	SubscriptionID string
}

func (a Azure) NewCreds() (*azidentity.DefaultAzureCredential, error) {
	if len(a.SubscriptionID) == 0 {
		return nil, errors.New("environment variable AZURE_SUBSCRIPTION_ID is not set")
	}

	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create default Azure credentials: %w", err)
	}

	return cred, nil
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

func (a Azure) NewStorageClient() (*armstorage.BlobContainersClient, error) {
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
