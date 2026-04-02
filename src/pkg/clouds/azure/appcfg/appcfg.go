package appcfg

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/data/azappconfig"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appconfiguration/armappconfiguration"
	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/clouds/azure"
	"github.com/DefangLabs/defang/src/pkg/term"
)

const storeNamePrefix = "defangcfg"

// AppConfiguration wraps an Azure App Configuration store.
type AppConfiguration struct {
	azure.Azure
	resourceGroupName string
	StoreName         string
	connectionString  string // read-write access key for the data plane
}

func New(resourceGroupName string, loc azure.Location, subscriptionID string) *AppConfiguration {
	return &AppConfiguration{
		Azure: azure.Azure{
			Location:       loc,
			SubscriptionID: subscriptionID,
		},
		resourceGroupName: resourceGroupName,
	}
}

func (a *AppConfiguration) newDataClient() (*azappconfig.Client, error) {
	if a.connectionString == "" {
		return nil, errors.New("App Configuration store not set up")
	}
	return azappconfig.NewClientFromConnectionString(a.connectionString, nil)
}

// fetchConnectionString retrieves the read-write access key for the store.
func (a *AppConfiguration) fetchConnectionString(ctx context.Context, client *armappconfiguration.ConfigurationStoresClient) error {
	pager := client.NewListKeysPager(a.resourceGroupName, a.StoreName, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("failed to list App Configuration keys: %w", err)
		}
		for _, key := range page.Value {
			if key.ConnectionString != nil && (key.ReadOnly == nil || !*key.ReadOnly) {
				a.connectionString = *key.ConnectionString
				return nil
			}
		}
	}
	return fmt.Errorf("no read-write access key found for App Configuration store %s", a.StoreName)
}

// SetUp creates the App Configuration store if it doesn't already exist in the resource group,
// fetches its read-write connection string, and populates StoreName and connectionString.
func (a *AppConfiguration) SetUp(ctx context.Context) error {
	cred, err := a.NewCreds()
	if err != nil {
		return err
	}

	client, err := armappconfiguration.NewConfigurationStoresClient(a.SubscriptionID, cred, nil)
	if err != nil {
		return err
	}

	// Look for an existing store in the resource group.
	pager := client.NewListByResourceGroupPager(a.resourceGroupName, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("failed to list App Configuration stores: %w", err)
		}
		for _, store := range page.Value {
			if store.Name != nil && strings.HasPrefix(*store.Name, storeNamePrefix) {
				a.StoreName = *store.Name
				term.Debugf("Using existing App Configuration store %s", a.StoreName)
				return a.fetchConnectionString(ctx, client)
			}
		}
	}

	// None found — create one.
	a.StoreName = storeNamePrefix + pkg.RandomID()
	term.Debugf("Creating App Configuration store %s", a.StoreName)
	poller, err := client.BeginCreate(ctx, a.resourceGroupName, a.StoreName, armappconfiguration.ConfigurationStore{
		Location: a.Location.Ptr(),
		SKU:      &armappconfiguration.SKU{Name: to.Ptr("Free")},
	}, nil)
	if err != nil {
		return fmt.Errorf("failed to create App Configuration store: %w", err)
	}
	if _, err := poller.PollUntilDone(ctx, nil); err != nil {
		return fmt.Errorf("failed to poll App Configuration store creation: %w", err)
	}
	return a.fetchConnectionString(ctx, client)
}

// PutSetting creates or updates a key in the App Configuration store.
func (a *AppConfiguration) PutSetting(ctx context.Context, key, value string) error {
	client, err := a.newDataClient()
	if err != nil {
		return err
	}
	_, err = client.SetSetting(ctx, key, &value, nil)
	return err
}

// DeleteSetting removes a key from the App Configuration store.
func (a *AppConfiguration) DeleteSetting(ctx context.Context, key string) error {
	client, err := a.newDataClient()
	if err != nil {
		return err
	}
	_, err = client.DeleteSetting(ctx, key, nil)
	return err
}

// ListSettings returns all keys with the given prefix, with the prefix stripped.
func (a *AppConfiguration) ListSettings(ctx context.Context, keyPrefix string) ([]string, error) {
	client, err := a.newDataClient()
	if err != nil {
		return nil, err
	}

	pager := client.NewListSettingsPager(azappconfig.SettingSelector{
		KeyFilter: to.Ptr(keyPrefix + "*"),
	}, nil)

	var keys []string
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list settings: %w", err)
		}
		for _, setting := range page.Settings {
			if setting.Key != nil {
				keys = append(keys, strings.TrimPrefix(*setting.Key, keyPrefix))
			}
		}
	}
	return keys, nil
}
