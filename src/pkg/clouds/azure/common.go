package azure

import (
	"errors"
	"fmt"
	"os"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage"
)

type Azure struct {
	Location Location
}

func NewCreds() (string, *azidentity.DefaultAzureCredential, error) {
	subscriptionID := os.Getenv("AZURE_SUBSCRIPTION_ID")
	if len(subscriptionID) == 0 {
		return "", nil, errors.New("environment variable AZURE_SUBSCRIPTION_ID is not set")
	}

	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return "", nil, fmt.Errorf("failed to create default Azure credentials: %w", err)
	}

	return subscriptionID, cred, nil
}

func NewStorageAccountsClient() (*armstorage.AccountsClient, error) {
	subscriptionID, cred, err := NewCreds()
	if err != nil {
		return nil, err
	}

	storageClient, err := armstorage.NewClientFactory(subscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create storage client: %w", err)
	}

	return storageClient.NewAccountsClient(), nil
}
