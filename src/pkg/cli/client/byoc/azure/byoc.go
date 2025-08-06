package azure

import (
	"context"
	"errors"

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

func NewByocProvider(ctx context.Context, tenantName types.TenantName) *ByocAzure {
	b := &ByocAzure{
		driver: aci.NewContainerInstance("defang-cd", ""), // default location => from AZURE_LOCATION env var
	}
	b.ByocBaseClient = byoc.NewByocBaseClient(ctx, tenantName, b)
	return b
}

// AccountInfo implements client.Provider.
func (b *ByocAzure) AccountInfo(context.Context) (*client.AccountInfo, error) {
	return &client.AccountInfo{
		AccountID: b.driver.SubscriptionID,
		Provider:  client.ProviderAzure,
		Region:    b.driver.Location.String(),
	}, nil
}

// BootstrapCommand implements client.Provider.
func (b *ByocAzure) BootstrapCommand(context.Context, client.BootstrapCommandRequest) (types.ETag, error) {
	return "", errors.ErrUnsupported
}

// BootstrapList implements client.Provider.
func (b *ByocAzure) BootstrapList(context.Context) ([]string, error) {
	return nil, errors.ErrUnsupported
}

// CreateUploadURL implements client.Provider.
func (b *ByocAzure) CreateUploadURL(ctx context.Context, req *defangv1.UploadURLRequest) (*defangv1.UploadURLResponse, error) {
	url, err := b.driver.CreateUploadURL(ctx, req.Digest)
	if err != nil {
		return nil, err
	}

	return &defangv1.UploadURLResponse{
		Url: url,
	}, nil
}

// Delete implements client.Provider.
func (b *ByocAzure) Delete(context.Context, *defangv1.DeleteRequest) (*defangv1.DeleteResponse, error) {
	return nil, errors.ErrUnsupported
}

// DeleteConfig implements client.Provider.
func (b *ByocAzure) DeleteConfig(context.Context, *defangv1.Secrets) error {
	return errors.ErrUnsupported
}

// Deploy implements client.Provider.
func (b *ByocAzure) Deploy(ctx context.Context, req *defangv1.DeployRequest) (*defangv1.DeployResponse, error) {
	// return nil, errors.ErrUnsupported
	return &defangv1.DeployResponse{}, byoc.DebugPulumi(ctx, nil, "up", "payload")
}

// Destroy implements client.Provider.
func (b *ByocAzure) Destroy(context.Context, *defangv1.DestroyRequest) (types.ETag, error) {
	return "", errors.ErrUnsupported
}

// GetDeploymentStatus implements client.Provider.
func (b *ByocAzure) GetDeploymentStatus(context.Context) error {
	return errors.ErrUnsupported
}

// GetProjectUpdate implements client.Provider.
func (b *ByocAzure) GetProjectUpdate(context.Context, string) (*defangv1.ProjectUpdate, error) {
	return nil, errors.ErrUnsupported
}

// GetService implements client.Provider.
func (b *ByocAzure) GetService(context.Context, *defangv1.GetRequest) (*defangv1.ServiceInfo, error) {
	return nil, errors.ErrUnsupported
}

// GetServices implements client.Provider.
func (b *ByocAzure) GetServices(context.Context, *defangv1.GetServicesRequest) (*defangv1.GetServicesResponse, error) {
	return nil, errors.ErrUnsupported
}

// ListConfig implements client.Provider.
func (b *ByocAzure) ListConfig(context.Context, *defangv1.ListConfigsRequest) (*defangv1.Secrets, error) {
	// return nil, errors.ErrUnsupported
	return &defangv1.Secrets{}, nil
}

// PrepareDomainDelegation implements client.Provider.
func (b *ByocAzure) PrepareDomainDelegation(context.Context, client.PrepareDomainDelegationRequest) (*client.PrepareDomainDelegationResponse, error) {
	return nil, nil // TODO: implement domain delegation for Azure
}

// Preview implements client.Provider.
func (b *ByocAzure) Preview(context.Context, *defangv1.DeployRequest) (*defangv1.DeployResponse, error) {
	return nil, errors.ErrUnsupported
}

// PutConfig implements client.Provider.
func (b *ByocAzure) PutConfig(context.Context, *defangv1.PutConfigRequest) error {
	return errors.ErrUnsupported
}

// QueryForDebug implements client.Provider.
func (b *ByocAzure) QueryForDebug(context.Context, *defangv1.DebugRequest) error {
	return errors.ErrUnsupported
}

// QueryLogs implements client.Provider.
func (b *ByocAzure) QueryLogs(context.Context, *defangv1.TailRequest) (client.ServerStream[defangv1.TailResponse], error) {
	return nil, errors.ErrUnsupported
}

// RemoteProjectName implements client.Provider.
// Subtle: this method shadows the method (*ByocBaseClient).RemoteProjectName of ByocAzure.ByocBaseClient.
func (b *ByocAzure) RemoteProjectName(context.Context) (string, error) {
	return "", errors.ErrUnsupported
}

// ServiceDNS implements client.Provider.
// Subtle: this method shadows the method (*ByocBaseClient).ServiceDNS of ByocAzure.ByocBaseClient.
func (b *ByocAzure) ServiceDNS(host string) string {
	return host
}

// SetCanIUseConfig implements client.Provider.
// Subtle: this method shadows the method (*ByocBaseClient).SetCanIUseConfig of ByocAzure.ByocBaseClient.
func (b *ByocAzure) SetCanIUseConfig(*defangv1.CanIUseResponse) {
	// return nil, errors.ErrUnsupported
}

// Subscribe implements client.Provider.
func (b *ByocAzure) Subscribe(context.Context, *defangv1.SubscribeRequest) (client.ServerStream[defangv1.SubscribeResponse], error) {
	return nil, errors.ErrUnsupported
}

// TearDown implements client.Provider.
func (b *ByocAzure) TearDown(ctx context.Context) error {
	return b.driver.TearDown(ctx)
}
