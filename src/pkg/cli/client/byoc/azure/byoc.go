package azure

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"iter"
	"net/http"
	"os"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerinstance/armcontainerinstance/v2"
	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc/state"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	cloudazure "github.com/DefangLabs/defang/src/pkg/clouds/azure"
	"github.com/DefangLabs/defang/src/pkg/clouds/azure/aci"
	defanghttp "github.com/DefangLabs/defang/src/pkg/http"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"google.golang.org/protobuf/proto"
)

type ByocAzure struct {
	*byoc.ByocBaseClient

	driver           *aci.ContainerInstance
	cdContainerGroup aci.ContainerGroupName
}

var _ client.Provider = (*ByocAzure)(nil)

func NewByocProvider(ctx context.Context, tenantLabel types.TenantLabel, stack string) *ByocAzure {
	b := &ByocAzure{
		driver: aci.NewContainerInstance("defang-cd", ""), // default location => from AZURE_LOCATION env var
	}
	b.ByocBaseClient = byoc.NewByocBaseClient(tenantLabel, b, stack)
	return b
}

func (b *ByocAzure) Driver() string {
	return "azure"
}

// SetUpCD implements client.Provider.
func (b *ByocAzure) SetUpCD(context.Context, bool) error {
	// return fmt.Errorf("SetUpCD: %w", errors.ErrUnsupported)
	term.Debugf("SetUpCD: no-op for Azure; CD environment will be set up on demand during Deploy")
	return nil
}

// CdCommand implements byoc.ProjectBackend.
func (b *ByocAzure) CdCommand(context.Context, client.CdCommandRequest) (types.ETag, error) {
	return "", fmt.Errorf("CdCommand: %w", errors.ErrUnsupported)
}

// CdList implements byoc.ProjectBackend.
func (b *ByocAzure) CdList(ctx context.Context, _ bool) (iter.Seq[state.Info], error) {
	if err := b.setUp(ctx); err != nil {
		return nil, err
	}

	blobs, err := b.driver.IterateBlobs(ctx, ".pulumi/stacks/")
	if err != nil {
		return nil, err
	}

	return func(yield func(state.Info) bool) {
		for item, err := range blobs {
			if err != nil {
				term.Debugf("Error iterating blobs: %v", err)
				return
			}
			st, err := state.ParsePulumiStateFile(ctx, item, b.driver.BlobContainerName, func(ctx context.Context, _, blobName string) ([]byte, error) {
				return b.driver.DownloadBlob(ctx, blobName)
			})
			if err != nil {
				term.Debugf("Skipping %q: %v", item.Name(), err)
				continue
			}
			if st == nil {
				continue
			}
			if !yield(state.Info{
				Project:   st.Project,
				Stack:     st.Name,
				Workspace: string(st.Workspace),
				CdRegion:  b.driver.Location.String(),
			}) {
				return
			}
		}
	}, nil
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
	return nil, fmt.Errorf("Delete: %w", errors.ErrUnsupported)
}

// DeleteConfig implements client.Provider.
func (b *ByocAzure) DeleteConfig(context.Context, *defangv1.Secrets) error {
	return fmt.Errorf("DeleteConfig: %w", errors.ErrUnsupported)
}

func (b *ByocAzure) setUp(ctx context.Context) error {
	// Lazily initialize location from AZURE_LOCATION env var (set by LoadStackEnv from the stack file).
	// Azure SDK does not natively support AZURE_LOCATION, so we handle it ourselves.
	if b.driver.Location == "" {
		loc := cloudazure.Location(os.Getenv("AZURE_LOCATION"))
		if loc == "" {
			return errors.New("AZURE_LOCATION is not set; please ensure your stack includes the Azure region")
		}
		b.driver.SetLocation(loc)
	}
	// Similarly, AZURE_SUBSCRIPTION_ID may be set by LoadStackEnv after construction.
	if b.driver.SubscriptionID == "" {
		b.driver.SubscriptionID = os.Getenv("AZURE_SUBSCRIPTION_ID")
	}
	if err := b.driver.SetUpResourceGroup(ctx); err != nil {
		return err
	}

	b.driver.ContainerGroupProps = &armcontainerinstance.ContainerGroupPropertiesProperties{
		OSType:        to.Ptr(armcontainerinstance.OperatingSystemTypesLinux), // TODO: from Platform
		RestartPolicy: to.Ptr(armcontainerinstance.ContainerGroupRestartPolicyNever),
	}
	if username := os.Getenv("DOCKERHUB_USERNAME"); username != "" {
		b.driver.ContainerGroupProps.ImageRegistryCredentials = append(b.driver.ContainerGroupProps.ImageRegistryCredentials, &armcontainerinstance.ImageRegistryCredential{
			Server:   to.Ptr("index.docker.io"),
			Username: to.Ptr(username),
			Password: to.Ptr(pkg.Getenv("DOCKERHUB_TOKEN", os.Getenv("DOCKERHUB_PASSWORD"))),
		})
	}

	if _, err := b.driver.SetUpStorageAccount(ctx); err != nil {
		return fmt.Errorf("failed to set up storage account: %w", err)
	}
	return nil
}

// Deploy implements client.Provider.
func (b *ByocAzure) Deploy(ctx context.Context, req *client.DeployRequest) (*defangv1.DeployResponse, error) {
	return b.deploy(ctx, req, "up")
}

func (b *ByocAzure) deploy(ctx context.Context, req *client.DeployRequest, verb string) (*defangv1.DeployResponse, error) {
	if b.CDImage == "" {
		return nil, errors.New("CD image is not set; please set the DEFANG_CD_IMAGE environment variable")
	}

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

	var payload string
	if len(data) < 1000 {
		payload = base64.StdEncoding.EncodeToString(data)
	} else {
		uploadURL, err := b.driver.CreateUploadURL(ctx, etag)
		if err != nil {
			return nil, err
		}
		resp, err := defanghttp.PutWithHeader(ctx, uploadURL, http.Header{
			"Content-Type":   []string{"application/protobuf"},
			"x-ms-blob-type": []string{"BlockBlob"},
		}, bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 && resp.StatusCode != 201 {
			return nil, fmt.Errorf("unexpected status code during upload: %s", resp.Status)
		}
		payload = defanghttp.RemoveQueryParam(uploadURL)
	}

	// Allow local debug run when DEFANG_PULUMI_DIR is set; returns ErrLocalPulumiStopped when run locally.
	if err := byoc.DebugPulumiNodeJS(ctx, env, verb, payload); err != nil {
		return &defangv1.DeployResponse{Etag: etag, Services: serviceInfos}, err
	}

	// Convert KEY=VALUE slice to map for ACI Run
	envMap := make(map[string]string, len(env))
	for _, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		envMap[k] = v
	}

	containers := []*armcontainerinstance.Container{
		{
			Name: to.Ptr("defang-cd"),
			Properties: &armcontainerinstance.ContainerProperties{
				Image: to.Ptr(b.CDImage),
				Resources: &armcontainerinstance.ResourceRequirements{
					Requests: &armcontainerinstance.ResourceRequests{
						CPU:        to.Ptr(2.0),
						MemoryInGB: to.Ptr(8.0),
					},
				},
			},
		},
	}

	taskID, err := b.driver.Run(ctx, containers, envMap, "node", "lib/index.js", verb, payload)
	if err != nil {
		return nil, err
	}
	b.cdContainerGroup = taskID
	return &defangv1.DeployResponse{Etag: etag, Services: serviceInfos}, nil
}

// Destroy implements client.Provider.
func (b *ByocAzure) Destroy(context.Context, *defangv1.DestroyRequest) (types.ETag, error) {
	return "", fmt.Errorf("Destroy: %w", errors.ErrUnsupported)
}

// GetDeploymentStatus implements client.Provider.
func (b *ByocAzure) GetDeploymentStatus(ctx context.Context) (bool, error) {
	done, err := b.driver.GetContainerGroupStatus(ctx, b.cdContainerGroup)
	if err != nil {
		return done, client.ErrDeploymentFailed{Message: err.Error()}
	}
	return done, nil
}

// GetPrivateDomain implements byoc.ProjectBackend.
func (b *ByocAzure) GetPrivateDomain(projectName string) string {
	panic("unimplemented")
}

// GetProjectUpdate implements byoc.ProjectBackend.
func (b *ByocAzure) GetProjectUpdate(ctx context.Context, projectName string) (*defangv1.ProjectUpdate, error) {
	if projectName == "" {
		return nil, client.ErrNotExist
	}
	if err := b.setUp(ctx); err != nil {
		return nil, err
	}

	path := b.GetProjectUpdatePath(projectName)
	term.Debug("Getting project update from blob:", b.driver.BlobContainerName, path)
	pbBytes, err := b.driver.DownloadBlob(ctx, path)
	if err != nil {
		var respErr *azcore.ResponseError
		if errors.As(err, &respErr) && respErr.StatusCode == 404 {
			return nil, client.ErrNotExist // no services yet
		}
		return nil, err
	}

	var projUpdate defangv1.ProjectUpdate
	if err := proto.Unmarshal(pbBytes, &projUpdate); err != nil {
		return nil, err
	}
	return &projUpdate, nil
}

// GetService implements client.Provider.
func (b *ByocAzure) GetService(context.Context, *defangv1.GetRequest) (*defangv1.ServiceInfo, error) {
	return nil, fmt.Errorf("GetService: %w", errors.ErrUnsupported)
}

// GetServices implements client.Provider.
func (b *ByocAzure) GetServices(context.Context, *defangv1.GetServicesRequest) (*defangv1.GetServicesResponse, error) {
	return nil, fmt.Errorf("GetServices: %w", errors.ErrUnsupported)
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
	return fmt.Errorf("PutConfig: %w", errors.ErrUnsupported)
}

// QueryLogs implements client.Provider.
func (b *ByocAzure) QueryLogs(context.Context, *defangv1.TailRequest) (iter.Seq2[*defangv1.TailResponse, error], error) {
	return nil, fmt.Errorf("QueryLogs: %w", errors.ErrUnsupported)
}

// RemoteProjectName implements client.Provider.
// Subtle: this method shadows the method (*ByocBaseClient).RemoteProjectName of ByocAzure.ByocBaseClient.
func (b *ByocAzure) RemoteProjectName(context.Context) (string, error) {
	return "", fmt.Errorf("RemoteProjectName: %w", errors.ErrUnsupported)
}

// ServiceDNS implements client.Provider.
// Subtle: this method shadows the method (*ByocBaseClient).ServiceDNS of ByocAzure.ByocBaseClient.
func (b *ByocAzure) ServiceDNS(host string) string {
	return host
}

// Subscribe implements client.Provider.
func (b *ByocAzure) Subscribe(context.Context, *defangv1.SubscribeRequest) (iter.Seq2[*defangv1.SubscribeResponse, error], error) {
	return nil, fmt.Errorf("Subscribe: %w", errors.ErrUnsupported)
}

// TearDown implements client.Provider.
func (b *ByocAzure) TearDown(ctx context.Context) error {
	return b.driver.TearDown(ctx)
}

// TearDownCD implements client.Provider.
func (b *ByocAzure) TearDownCD(context.Context) error {
	return fmt.Errorf("TearDownCD: %w", errors.ErrUnsupported)
}

// UpdateShardDomain implements client.DNSResolver.
func (b *ByocAzure) UpdateShardDomain(context.Context) error {
	return fmt.Errorf("UpdateShardDomain: %w", errors.ErrUnsupported)
}
