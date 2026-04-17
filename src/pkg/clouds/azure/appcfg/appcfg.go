package appcfg

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/data/azappconfig"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appconfiguration/armappconfiguration"
	"github.com/DefangLabs/defang/src/pkg/clouds/azure"
	"github.com/DefangLabs/defang/src/pkg/term"
)

// storeNameSuffixLen is the length in hex chars of the uniqueness suffix appended to the
// App Configuration store name. 12 hex chars = 48 bits of entropy, plenty to avoid
// collisions across projects/subscriptions.
const storeNameSuffixLen = 12

// StoreName returns the deterministic Azure App Configuration store name for the given
// resource group in the given subscription.
//
// App Configuration store names must be globally unique across Azure (5–50 chars,
// alphanumeric + hyphens). The name is built as {resource-group}-{suffix} where the
// suffix is a hash of (subscription_id, resource_group). Hashing both inputs guarantees
// the suffix differs between projects/subscriptions even if the resource-group portion
// has to be truncated to fit the 50-char limit.
//
// Both the CLI (when creating the store) and the Pulumi provider (when looking it up)
// call this function, so neither side needs to pass the name to the other.
func StoreName(resourceGroupName, subscriptionID string) string {
	h := sha256.Sum256([]byte(subscriptionID + "|" + resourceGroupName))
	suffix := hex.EncodeToString(h[:])[:storeNameSuffixLen]

	name := resourceGroupName + "-" + suffix
	if len(name) > 50 {
		name = name[:50-1-len(suffix)] + "-" + suffix
	}
	return name
}

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

// SetUp creates the App Configuration store (using the deterministic StoreName) if it
// doesn't already exist, fetches its read-write connection string, and populates
// StoreName and connectionString.
func (a *AppConfiguration) SetUp(ctx context.Context) error {
	cred, err := a.NewCreds()
	if err != nil {
		return err
	}

	client, err := armappconfiguration.NewConfigurationStoresClient(a.SubscriptionID, cred, nil)
	if err != nil {
		return err
	}

	a.StoreName = StoreName(a.resourceGroupName, a.SubscriptionID)
	term.Debugf("Creating or updating App Configuration store %s", a.StoreName)
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
