package azure

import (
	"context"
	"iter"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc/state"
	"github.com/DefangLabs/defang/src/pkg/clouds/azure/aci"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type ByocAzure struct {
	*byoc.ByocBaseClient

	driver *aci.ContainerInstance
}

var _ client.Provider = (*ByocAzure)(nil)

func NewByocAzure(ctx context.Context, tenantLabel types.TenantLabel, stack string) *ByocAzure {
	b := &ByocAzure{
		driver: aci.NewContainerInstance("defang-cd", ""), // default location => from AZURE_LOCATION env var
	}
	b.ByocBaseClient = byoc.NewByocBaseClient(tenantLabel, b, stack)
	return b
}

// CdCommand implements byoc.ProjectBackend.
func (b *ByocAzure) CdCommand(context.Context, client.CdCommandRequest) (types.ETag, error) {
	return &client.AccountInfo{
		AccountID: b.driver.SubscriptionID,
		Provider:  client.ProviderAzure,
		Region:    b.driver.Location.String(),
	}, nil
}

// CdList implements byoc.ProjectBackend.
func (b *ByocAzure) CdList(context.Context, bool) (iter.Seq[state.Info], error) {
	panic("unimplemented")
}

// AccountInfo implements client.Provider.
func (b *ByocAzure) AccountInfo(context.Context) (*client.AccountInfo, error) {
	panic("unimplemented")
}

// CreateUploadURL implements client.Provider.
func (b *ByocAzure) CreateUploadURL(context.Context, *defangv1.UploadURLRequest) (*defangv1.UploadURLResponse, error) {
	panic("unimplemented")
}

// DeleteConfig implements client.Provider.
func (b *ByocAzure) DeleteConfig(context.Context, *defangv1.Secrets) error {
	panic("unimplemented")
}

// Deploy implements client.Provider.
func (b *ByocAzure) Deploy(context.Context, *client.DeployRequest) (*defangv1.DeployResponse, error) {
	panic("unimplemented")
}

// GetDeploymentStatus implements client.Provider.
func (b *ByocAzure) GetDeploymentStatus(context.Context) (bool, error) {
	panic("unimplemented")
}

// GetPrivateDomain implements byoc.ProjectBackend.
func (b *ByocAzure) GetPrivateDomain(projectName string) string {
	panic("unimplemented")
}

// GetProjectUpdate implements byoc.ProjectBackend.
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
func (b *ByocAzure) Preview(context.Context, *client.DeployRequest) (*defangv1.DeployResponse, error) {
	panic("unimplemented")
}

// PutConfig implements client.Provider.
func (b *ByocAzure) PutConfig(context.Context, *defangv1.PutConfigRequest) error {
	panic("unimplemented")
}

// QueryLogs implements client.Provider.
func (b *ByocAzure) QueryLogs(context.Context, *defangv1.TailRequest) (iter.Seq2[*defangv1.TailResponse, error], error) {
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

// SetUpCD implements client.Provider.
func (b *ByocAzure) SetUpCD(context.Context) error {
	panic("unimplemented")
}

// Subscribe implements client.Provider.
func (b *ByocAzure) Subscribe(context.Context, *defangv1.SubscribeRequest) (iter.Seq2[*defangv1.SubscribeResponse, error], error) {
	panic("unimplemented")
}

// TearDown implements client.Provider.
func (b *ByocAzure) TearDown(ctx context.Context) error {
	return b.driver.TearDown(ctx)
}

// TearDownCD implements client.Provider.
func (b *ByocAzure) TearDownCD(context.Context) error {
	panic("unimplemented")
}

// UpdateShardDomain implements client.DNSResolver.
func (b *ByocAzure) UpdateShardDomain(context.Context) error {
	panic("unimplemented")
}
