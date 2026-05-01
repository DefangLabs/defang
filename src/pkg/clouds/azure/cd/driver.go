package cd

import (
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources/v2"
	"github.com/DefangLabs/defang/src/pkg/clouds/azure"
)

type Driver struct {
	azure.Azure
	resourceGroupPrefix string
	resourceGroupName   string
	StorageAccount      string
	BlobContainerName   string
}

func New(resourceGroupPrefix string, location azure.Location) *Driver {
	d := &Driver{
		Azure: azure.Azure{
			Location: location,
		},
		resourceGroupPrefix: resourceGroupPrefix,
	}
	d.resourceGroupName = resourceGroupPrefix + "-" + location.String()
	return d
}

func (d *Driver) ResourceGroupName() string {
	return d.resourceGroupName
}

// SetLocation updates the location and recomputes the resource group name.
func (d *Driver) SetLocation(loc azure.Location) {
	d.Location = loc
	d.resourceGroupName = d.resourceGroupPrefix + "-" + loc.String()
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
