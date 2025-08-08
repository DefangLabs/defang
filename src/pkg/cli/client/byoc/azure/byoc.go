package azure

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"iter"
	"os"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc/state"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/clouds"
	"github.com/DefangLabs/defang/src/pkg/clouds/azure/aci"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"google.golang.org/protobuf/proto"
)

type ByocAzure struct {
	*byoc.ByocBaseClient

	driver *aci.ContainerInstance
}

var _ client.Provider = (*ByocAzure)(nil)

func NewByocProvider(ctx context.Context, tenantLabel types.TenantLabel, stack string) *ByocAzure {
	b := &ByocAzure{
		driver: aci.NewContainerInstance("defang-cd", ""), // default location => from AZURE_LOCATION env var
	}
	b.ByocBaseClient = byoc.NewByocBaseClient(tenantLabel, b, stack)
	return b
}

// SetUpCD implements client.Provider.
func (b *ByocAzure) SetUpCD(context.Context) error {
	return errors.ErrUnsupported
}

// CdCommand implements byoc.ProjectBackend.
func (b *ByocAzure) CdCommand(context.Context, client.CdCommandRequest) (types.ETag, error) {
	return "", errors.ErrUnsupported
}

// CdList implements byoc.ProjectBackend.
func (b *ByocAzure) CdList(context.Context, bool) (iter.Seq[state.Info], error) {
	return nil, errors.ErrUnsupported
}

// AccountInfo implements client.Provider.
func (b *ByocAzure) AccountInfo(context.Context) (*client.AccountInfo, error) {
	return &client.AccountInfo{
		AccountID: b.driver.SubscriptionID,
		Provider:  client.ProviderAzure,
		Region:    b.driver.Location.String(),
	}, nil
}

// CreateUploadURL implements client.Provider.
func (b *ByocAzure) CreateUploadURL(ctx context.Context, req *defangv1.UploadURLRequest) (*defangv1.UploadURLResponse, error) {
	if err := b.setUp(ctx); err != nil {
		return nil, err
	}

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

func (b *ByocAzure) setUp(ctx context.Context) error {
	return b.driver.SetUp(ctx, []clouds.Container{
		{
			Name:  "defang-cd",
			Image: b.CDImage,
		},
	})
}

// Deploy implements client.Provider.
func (b *ByocAzure) Deploy(ctx context.Context, req *client.DeployRequest) (*defangv1.DeployResponse, error) {
	return b.deploy(ctx, req, "up")
}

func (b *ByocAzure) deploy(ctx context.Context, req *client.DeployRequest, verb string) (*defangv1.DeployResponse, error) {
	// If multiple Compose files were provided, req.Compose is the merged representation of all the files
	project, err := compose.LoadFromContent(ctx, req.Compose, "")
	if err != nil {
		return nil, err
	}

	if err := b.setUp(ctx); err != nil {
		return nil, err
	}

	etag := pkg.RandomID()
	serviceInfos, err := b.GetServiceInfos(ctx, project.Name, req.DelegateDomain, etag, project.Services)
	if err != nil {
		return nil, err
	}

	data, err := proto.Marshal(&defangv1.ProjectUpdate{
		CdVersion: b.CDImage,
		Compose:   req.Compose,
		Services:  serviceInfos,
	})
	if err != nil {
		return nil, err
	}

	// From https://www.pulumi.com/docs/iac/concepts/state-and-backends/#azure-blob-storage
	defangStateUrl := fmt.Sprintf(`azblob://%s?storage_account=%s`, b.driver.BlobContainerName, b.driver.StorageAccount)
	pulumiBackendKey, pulumiBackendValue, err := byoc.GetPulumiBackend(defangStateUrl)
	if err != nil {
		return nil, err
	}
	env := []string{
		"AZURE_LOCATION=" + b.driver.Location.String(),
		"AZURE_SUBSCRIPTION_ID=" + b.driver.SubscriptionID,
		"DEFANG_DEBUG=" + os.Getenv("DEFANG_DEBUG"), // TODO: use the global DoDebug flag
		"DEFANG_JSON=" + os.Getenv("DEFANG_JSON"),
		"DEFANG_ORG=" + string(b.TenantLabel),
		"DEFANG_PREFIX=" + b.Prefix,
		"DEFANG_STATE_URL=" + defangStateUrl,
		"NPM_CONFIG_UPDATE_NOTIFIER=" + "false",
		// "PRIVATE_DOMAIN=" + byoc.GetPrivateDomain(projectName), TODO: implement
		"PROJECT=" + project.Name,                                 // may be empty
		pulumiBackendKey + "=" + pulumiBackendValue,               // TODO: make secret
		"PULUMI_CONFIG_PASSPHRASE=" + byoc.PulumiConfigPassphrase, // TODO: make secret
		"PULUMI_COPILOT=false",
		"PULUMI_SKIP_UPDATE_CHECK=true",
		"STACK=" + b.PulumiStack,
	}
	if !term.StdoutCanColor() {
		env = append(env, "NO_COLOR=1")
	}
	err = byoc.DebugPulumiNodeJS(ctx, env, verb, base64.StdEncoding.EncodeToString(data)) // TODO: handle large projects
	return &defangv1.DeployResponse{Etag: etag, Services: serviceInfos}, err
}

// Destroy implements client.Provider.
func (b *ByocAzure) Destroy(context.Context, *defangv1.DestroyRequest) (types.ETag, error) {
	return "", errors.ErrUnsupported
}

// GetDeploymentStatus implements client.Provider.
func (b *ByocAzure) GetDeploymentStatus(context.Context) (bool, error) {
	return false, errors.ErrUnsupported
}

// GetPrivateDomain implements byoc.ProjectBackend.
func (b *ByocAzure) GetPrivateDomain(projectName string) string {
	panic("unimplemented")
}

// GetProjectUpdate implements byoc.ProjectBackend.
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
func (b *ByocAzure) Preview(ctx context.Context, req *client.DeployRequest) (*defangv1.DeployResponse, error) {
	return b.deploy(ctx, req, "preview")
}

// PutConfig implements client.Provider.
func (b *ByocAzure) PutConfig(context.Context, *defangv1.PutConfigRequest) error {
	return errors.ErrUnsupported
}

// QueryLogs implements client.Provider.
func (b *ByocAzure) QueryLogs(context.Context, *defangv1.TailRequest) (iter.Seq2[*defangv1.TailResponse, error], error) {
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

// Subscribe implements client.Provider.
func (b *ByocAzure) Subscribe(context.Context, *defangv1.SubscribeRequest) (iter.Seq2[*defangv1.SubscribeResponse, error], error) {
	return nil, errors.ErrUnsupported
}

// TearDown implements client.Provider.
func (b *ByocAzure) TearDown(ctx context.Context) error {
	return b.driver.TearDown(ctx)
}

// TearDownCD implements client.Provider.
func (b *ByocAzure) TearDownCD(context.Context) error {
	return errors.ErrUnsupported
}

// UpdateShardDomain implements client.DNSResolver.
func (b *ByocAzure) UpdateShardDomain(context.Context) error {
	return errors.ErrUnsupported
}
