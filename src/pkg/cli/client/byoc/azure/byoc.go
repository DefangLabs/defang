package azure

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc"
	"github.com/DefangLabs/defang/src/pkg/clouds/azure/aci"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type ByocAzure struct {
	*byoc.ByocBaseClient

	driver *aci.ContainerInstance
}

var _ client.Provider = (*ByocAzure)(nil)

func NewByocAzure(ctx context.Context, tenantName types.TenantName) *ByocAzure {
	b := &ByocAzure{
		driver: aci.NewContainerInstance("defang-cd", ""), // default location => from AZURE_LOCATION env var
	}
	b.ByocBaseClient = byoc.NewByocBaseClient(ctx, tenantName, b)
	return b
}

// AccountInfo implements client.Provider.
func (b *ByocAzure) AccountInfo(context.Context) (*client.AccountInfo, error) {
	panic("unimplemented")
}

// BootstrapCommand implements client.Provider.
func (b *ByocAzure) BootstrapCommand(context.Context, client.BootstrapCommandRequest) (types.ETag, error) {
	panic("unimplemented")
}

// BootstrapList implements client.Provider.
func (b *ByocAzure) BootstrapList(context.Context) ([]string, error) {
	panic("unimplemented")
}

// CreateUploadURL implements client.Provider.
func (b *ByocAzure) CreateUploadURL(context.Context, *defangv1.UploadURLRequest) (*defangv1.UploadURLResponse, error) {
	panic("unimplemented")
}

// Delete implements client.Provider.
func (b *ByocAzure) Delete(context.Context, *defangv1.DeleteRequest) (*defangv1.DeleteResponse, error) {
	panic("unimplemented")
}

// DeleteConfig implements client.Provider.
func (b *ByocAzure) DeleteConfig(context.Context, *defangv1.Secrets) error {
	panic("unimplemented")
}

// Deploy implements client.Provider.
func (b *ByocAzure) Deploy(context.Context, *defangv1.DeployRequest) (*defangv1.DeployResponse, error) {
	panic("unimplemented")
}

// Destroy implements client.Provider.
func (b *ByocAzure) Destroy(context.Context, *defangv1.DestroyRequest) (types.ETag, error) {
	panic("unimplemented")
}

// GetDeploymentStatus implements client.Provider.
func (b *ByocAzure) GetDeploymentStatus(context.Context) error {
	panic("unimplemented")
}

// GetProjectUpdate implements client.Provider.
func (b *ByocAzure) GetProjectUpdate(context.Context, string) (*defangv1.ProjectUpdate, error) {
	panic("unimplemented")
}

// GetService implements client.Provider.
func (b *ByocAzure) GetService(context.Context, *defangv1.GetRequest) (*defangv1.ServiceInfo, error) {
	panic("unimplemented")
}

// GetServices implements client.Provider.
func (b *ByocAzure) GetServices(context.Context, *defangv1.GetServicesRequest) (*defangv1.GetServicesResponse, error) {
	panic("unimplemented")
}

// ListConfig implements client.Provider.
func (b *ByocAzure) ListConfig(context.Context, *defangv1.ListConfigsRequest) (*defangv1.Secrets, error) {
	panic("unimplemented")
}

// PrepareDomainDelegation implements client.Provider.
func (b *ByocAzure) PrepareDomainDelegation(context.Context, client.PrepareDomainDelegationRequest) (*client.PrepareDomainDelegationResponse, error) {
	panic("unimplemented")
}

// Preview implements client.Provider.
func (b *ByocAzure) Preview(context.Context, *defangv1.DeployRequest) (*defangv1.DeployResponse, error) {
	panic("unimplemented")
}

// PutConfig implements client.Provider.
func (b *ByocAzure) PutConfig(context.Context, *defangv1.PutConfigRequest) error {
	panic("unimplemented")
}

// QueryForDebug implements client.Provider.
func (b *ByocAzure) QueryForDebug(context.Context, *defangv1.DebugRequest) error {
	panic("unimplemented")
}

// QueryLogs implements client.Provider.
func (b *ByocAzure) QueryLogs(context.Context, *defangv1.TailRequest) (client.ServerStream[defangv1.TailResponse], error) {
	panic("unimplemented")
}

// RemoteProjectName implements client.Provider.
// Subtle: this method shadows the method (*ByocBaseClient).RemoteProjectName of ByocAzure.ByocBaseClient.
func (b *ByocAzure) RemoteProjectName(context.Context) (string, error) {
	panic("unimplemented")
}

// ServiceDNS implements client.Provider.
// Subtle: this method shadows the method (*ByocBaseClient).ServiceDNS of ByocAzure.ByocBaseClient.
func (b *ByocAzure) ServiceDNS(string) string {
	panic("unimplemented")
}

// SetCanIUseConfig implements client.Provider.
// Subtle: this method shadows the method (*ByocBaseClient).SetCanIUseConfig of ByocAzure.ByocBaseClient.
func (b *ByocAzure) SetCanIUseConfig(*defangv1.CanIUseResponse) {
	panic("unimplemented")
}

// Subscribe implements client.Provider.
func (b *ByocAzure) Subscribe(context.Context, *defangv1.SubscribeRequest) (client.ServerStream[defangv1.SubscribeResponse], error) {
	panic("unimplemented")
}

// TearDown implements client.Provider.
func (b *ByocAzure) TearDown(ctx context.Context) error {
	return b.driver.TearDown(ctx)
}
