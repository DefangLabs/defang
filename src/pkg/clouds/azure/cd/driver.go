package cd

import (
	"fmt"
	"sync"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources/v2"
	"github.com/DefangLabs/defang/src/pkg/clouds/azure"
)

type Driver struct {
	azure.Azure
	// resourceGroupName is the shared CD resource group (no location suffix).
	// One per subscription holds the CD task and Pulumi state for every
	// deployment regardless of target region.
	resourceGroupName string
	// cdLocation is the Azure region the CD resource group itself lives in
	// (the "primary" CD region — first-deploy-wins). It's resolved by
	// SetUpResourceGroup, either by reading an existing RG's location or by
	// creating a new RG using Location. Distinct from Location, which is the
	// per-call deploy target passed through to the CD task as AZURE_LOCATION.
	cdLocation        azure.Location
	storageKeyMu      sync.Mutex // guards storageKey
	storageKey        string
	StorageAccount    string
	BlobContainerName string
}

func New(resourceGroupName string, location azure.Location) *Driver {
	return &Driver{
		Azure:             azure.Azure{Location: location},
		resourceGroupName: resourceGroupName,
	}
}

func (d *Driver) ResourceGroupName() string {
	return d.resourceGroupName
}

// CdLocation returns the resolved primary CD region (set by SetUpResourceGroup).
// Returns empty until the RG has been resolved.
func (d *Driver) CdLocation() azure.Location {
	return d.cdLocation
}

// SetLocation updates the deploy-target location. CD infra location
// (resourceGroupName, cdLocation) is independent and resolved separately.
func (d *Driver) SetLocation(loc azure.Location) {
	d.Location = loc
}

func (d *Driver) newResourceGroupClient() (*armresources.ResourceGroupsClient, error) {
	cred, err := d.NewCreds()
	if err != nil {
		return nil, err
	}
	client, err := armresources.NewResourceGroupsClient(d.SubscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource group client: %w", err)
	}
	return client, nil
}
