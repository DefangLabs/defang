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
	"path/filepath"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc/state"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	cloudazure "github.com/DefangLabs/defang/src/pkg/clouds/azure"
	"github.com/DefangLabs/defang/src/pkg/clouds/azure/aca"
	"github.com/DefangLabs/defang/src/pkg/clouds/azure/acr"
	"github.com/DefangLabs/defang/src/pkg/clouds/azure/cd"
	"github.com/DefangLabs/defang/src/pkg/clouds/azure/keyvault"
	defanghttp "github.com/DefangLabs/defang/src/pkg/http"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/tokenstore"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type ByocAzure struct {
	*byoc.ByocBaseClient

	driver    *cd.Driver
	job       *aca.Job
	kv        *keyvault.KeyVault
	cdRunID   string
	cdEtag    string
	setUpDone bool // true once full setUp has completed; prevents redundant API calls
}

var _ client.Provider = (*ByocAzure)(nil)

func NewByocProvider(ctx context.Context, tenantLabel types.TenantLabel, stack string) *ByocAzure {
	b := &ByocAzure{
		driver: cd.New("defang-cd", ""), // default location => from AZURE_LOCATION env var
		job:    &aca.Job{},
	}
	b.ByocBaseClient = byoc.NewByocBaseClient(tenantLabel, b, stack)
	b.driver.TokenStore = &tokenstore.LocalDirTokenStore{Dir: filepath.Join(client.StateDir, "providers", "azure")}
	return b
}

func (b *ByocAzure) Driver() string {
	return "azure"
}

// SetUpCD implements client.Provider.
func (b *ByocAzure) SetUpCD(context.Context, bool) error {
	term.Debugf("SetUpCD: no-op for Azure; CD environment will be set up on demand during Deploy")
	return nil
}

// CdCommand implements byoc.ProjectBackend.
func (b *ByocAzure) CdCommand(ctx context.Context, req client.CdCommandRequest) (*client.CdCommandResponse, error) {
	if err := b.setUpForConfig(ctx, req.Project); err != nil {
		return nil, err
	}
	if err := b.setUp(ctx); err != nil {
		return nil, err
	}
	envMap, err := b.buildCdEnv(req.Project)
	if err != nil {
		return nil, err
	}
	if err := b.setUpJob(ctx, envMap); err != nil {
		return nil, err
	}
	etag := pkg.RandomID()
	execName, err := b.job.StartJobExecution(ctx, aca.JobRequest{
		Image:   b.CDImage,
		Command: []string{"/app/cd", string(req.Command)},
		Envs:    envMap,
		Timeout: 30 * time.Minute,
	})
	if err != nil {
		return nil, err
	}
	b.cdRunID = execName
	b.cdEtag = etag
	return &client.CdCommandResponse{
		CdId:   execName,
		CdType: defangv1.CdType_CD_TYPE_AZURE_ACI_JOBID,
		ETag:   etag,
	}, nil
}

// CdList implements byoc.ProjectBackend.
func (b *ByocAzure) CdList(ctx context.Context, _ bool) (iter.Seq[state.Info], error) {
	if err := b.setUp(ctx); err != nil {
		return nil, err
	}

	blobs, err := b.driver.IterateBlobsInContainer(ctx, cd.PulumiContainerName, ".pulumi/stacks/")
	if err != nil {
		return nil, err
	}

	return func(yield func(state.Info) bool) {
		for item, err := range blobs {
			if err != nil {
				term.Debugf("Error iterating blobs: %v", err)
				return
			}
			st, err := state.ParsePulumiStateFile(ctx, item, cd.PulumiContainerName, func(ctx context.Context, container, blobName string) ([]byte, error) {
				return b.driver.DownloadBlobFromContainer(ctx, container, blobName)
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

// DeleteConfig implements client.Provider. Read-only for discovery: if the
// Key Vault doesn't exist yet, there's nothing to delete — return success
// instead of provisioning it just to tear down.
func (b *ByocAzure) DeleteConfig(ctx context.Context, secrets *defangv1.Secrets) error {
	found, err := b.findForConfig(ctx, secrets.Project)
	if err != nil {
		return err
	}
	if !found {
		return nil // nothing configured yet, nothing to delete
	}
	for _, name := range secrets.Names {
		key := b.StackDir(secrets.Project, name)
		secretName := keyvault.ToSecretName(key)
		term.Debugf("Deleting Key Vault secret %q", secretName)
		if err := b.kv.DeleteSecret(ctx, secretName); err != nil {
			return fmt.Errorf("failed to delete Key Vault secret %q: %w", name, err)
		}
	}
	return nil
}

// setUpLocation lazily resolves AZURE_LOCATION and AZURE_SUBSCRIPTION_ID from the environment
// and syncs the values to the job. It makes no API calls.
func (b *ByocAzure) setUpLocation() error {
	if b.driver.Location == "" {
		loc := cloudazure.Location(os.Getenv("AZURE_LOCATION"))
		if loc == "" {
			return errors.New("AZURE_LOCATION is not set; please ensure your stack includes the Azure region")
		}
		b.driver.SetLocation(loc)
	}
	if b.driver.SubscriptionID == "" {
		b.driver.SubscriptionID = os.Getenv("AZURE_SUBSCRIPTION_ID")
	}
	b.job.Azure = b.driver.Azure
	b.job.ResourceGroup = b.driver.ResourceGroupName()
	return nil
}

// projectResourceGroupName returns the resource group name for project-specific resources
// (App Configuration store and deployed services).
// Format: defang-{project}-{stack}-{location}, e.g. "defang-myapp-test-westus2".
// This group is owned by one project+stack and is separate from the shared CD resource group.
func (b *ByocAzure) projectResourceGroupName(projectName string) string {
	return "defang-" + projectName + "-" + b.PulumiStack
}

// findForConfig binds to a pre-existing project Key Vault without creating
// anything new. Returns (true, nil) when the vault exists, (false, nil) when
// it or its resource group doesn't — which callers like ListConfig /
// DeleteConfig treat as "nothing configured yet".
//
// On a successful Find we also self-grant "Key Vault Secrets Officer" to the
// current caller (idempotent — RoleAssignmentExists is a no-op). This
// onboards new teammates onto a shared stack whose vault was created by
// someone else; without it, the read paths would 403 forever for them.
func (b *ByocAzure) findForConfig(ctx context.Context, projectName string) (bool, error) {
	if err := b.setUpLocation(); err != nil {
		return false, err
	}
	if b.kv != nil {
		return true, nil
	}
	rgName := b.projectResourceGroupName(projectName)
	kv := keyvault.New(rgName, b.driver.Azure)
	found, err := kv.Find(ctx)
	if err != nil {
		return false, err
	}
	if !found {
		return false, nil
	}
	if err := kv.EnsureSecretsOfficer(ctx); err != nil {
		return false, err
	}
	b.kv = kv
	return true, nil
}

// setUpForConfig creates the project-specific Key Vault (and the resource
// group that holds it) on first use. Idempotent. b.kv is only cached on
// successful SetUp, so a failed attempt doesn't mask the root cause for
// subsequent config operations within the same process.
func (b *ByocAzure) setUpForConfig(ctx context.Context, projectName string) error {
	if err := b.setUpLocation(); err != nil {
		return err
	}
	rgName := b.projectResourceGroupName(projectName)
	if err := b.driver.CreateResourceGroup(ctx, rgName); err != nil {
		return err
	}
	if b.kv == nil {
		kv := keyvault.New(rgName, b.driver.Azure)
		if err := kv.SetUp(ctx); err != nil {
			return err
		}
		b.kv = kv
	}
	return nil
}

// setUp sets up the shared CD infrastructure: resource group, blob storage, the Container
// Apps environment, and the job's managed identity. It does NOT create the CD job itself
// (SetUpJob must be called separately with env vars baked in) and does NOT set up
// project-specific resources (use setUpForConfig for App Configuration).
func (b *ByocAzure) setUp(ctx context.Context) error {
	if err := b.setUpLocation(); err != nil {
		return err
	}

	if b.setUpDone {
		return nil
	}

	// Create the shared CD resource group (defang-cd-{location}).
	if err := b.driver.SetUpResourceGroup(ctx); err != nil {
		return err
	}

	if _, err := b.driver.SetUpStorageAccount(ctx); err != nil {
		return fmt.Errorf("failed to set up storage account: %w", err)
	}

	if err := b.job.SetUpEnvironment(ctx); err != nil {
		return fmt.Errorf("failed to set up container apps environment: %w", err)
	}

	b.setUpDone = true
	return nil
}

// setUpJob creates/updates the CD job with the given env vars baked into its template,
// and grants the job's managed identity read access to the CD storage account. The job
// must already have SetUpEnvironment called on it (via setUp). The CD image is pulled
// anonymously — its registry must allow anonymous pull.
func (b *ByocAzure) setUpJob(ctx context.Context, envMap map[string]string) error {
	if b.CDImage == "" {
		return errors.New("CD image is not set; please set the DEFANG_CD_IMAGE environment variable")
	}
	if err := b.job.SetUpJob(ctx, b.CDImage, envMap); err != nil {
		return fmt.Errorf("failed to set up CD job: %w", err)
	}
	if err := b.job.SetUpManagedIdentity(ctx, b.driver.StorageAccount); err != nil {
		return fmt.Errorf("failed to set up managed identity: %w", err)
	}
	return nil
}

// buildCdEnv returns the environment map that every CD container run needs.
func (b *ByocAzure) buildCdEnv(projectName string) (map[string]string, error) {
	// Pulumi state lives in its own container (`pulumi`), separate from the
	// `uploads` container (etag payloads, tarballs) and the `projects`
	// container (project.pb audit blobs written by the CD task).
	defangStateUrl := fmt.Sprintf(`azblob://%s?storage_account=%s`, cd.PulumiContainerName, b.driver.StorageAccount)
	pulumiBackendKey, pulumiBackendValue, err := byoc.GetPulumiBackend(defangStateUrl)
	if err != nil {
		return nil, err
	}
	// AZURE_RESOURCE_GROUP and AZURE_KEY_VAULT_NAME are intentionally omitted:
	// the Pulumi Azure provider now derives both deterministically from
	// {project, stack, location, subscription} using the same formulas the
	// CLI uses (projectResourceGroupName, keyvault.VaultName), so passing
	// them as env vars would just be another spot for the two sides to
	// drift out of sync.
	env := map[string]string{
		"AZURE_LOCATION":             b.driver.Location.String(),
		"AZURE_SUBSCRIPTION_ID":      b.driver.SubscriptionID,
		"DEFANG_DEBUG":               os.Getenv("DEFANG_DEBUG"),
		"DEFANG_JSON":                os.Getenv("DEFANG_JSON"),
		"DEFANG_ORG":                 string(b.TenantLabel),
		"DEFANG_PREFIX":              b.Prefix,
		"DEFANG_PULUMI_DEBUG":        os.Getenv("DEFANG_PULUMI_DEBUG"),
		"DEFANG_PULUMI_DIFF":         os.Getenv("DEFANG_PULUMI_DIFF"),
		"DEFANG_STATE_URL":           defangStateUrl,
		"HOME":                       "/root", // TODO: should be in Dockerfile
		"NPM_CONFIG_UPDATE_NOTIFIER": "false",
		"PROJECT":                    projectName,
		pulumiBackendKey:             pulumiBackendValue, // TODO: make secret
		"PULUMI_AUTOMATION_API_SKIP_VERSION_CHECK": "true",
		"PULUMI_CONFIG_PASSPHRASE":                 byoc.PulumiConfigPassphrase, // TODO: make secret
		"PULUMI_COPILOT":                           "false",
		// "PULUMI_DIY_BACKEND_DISABLE_CHECKPOINT_BACKUPS": "true", TODO: use versioned bucket
		"PULUMI_SKIP_UPDATE_CHECK": "true",
		"STACK":                    b.PulumiStack,
		"USER":                     "root", // TODO: should be in Dockerfile
	}
	if targets := os.Getenv("DEFANG_PULUMI_TARGETS"); targets != "" {
		env["DEFANG_PULUMI_TARGETS"] = targets
	}
	if !term.StdoutCanColor() {
		env["NO_COLOR"] = "1"
	}
	return env, nil
}

// Deploy implements client.Provider.
func (b *ByocAzure) Deploy(ctx context.Context, req *client.DeployRequest) (*client.DeployResponse, error) {
	return b.deploy(ctx, req, "up")
}

func (b *ByocAzure) deploy(ctx context.Context, req *client.DeployRequest, verb string) (*client.DeployResponse, error) {
	if b.CDImage == "" {
		return nil, errors.New("CD image is not set; please set the DEFANG_CD_IMAGE environment variable")
	}

	// If multiple Compose files were provided, req.Compose is the merged representation of all the files
	project, err := compose.LoadFromContent(ctx, req.Compose, "")
	if err != nil {
		return nil, err
	}

	if err := b.setUpForConfig(ctx, project.Name); err != nil {
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

	envMap, err := b.buildCdEnv(project.Name)
	if err != nil {
		return nil, err
	}
	if err := b.setUpJob(ctx, envMap); err != nil {
		return nil, err
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
		payload = defanghttp.RemoveQueryParam(uploadURL) // managed identity provides blob read access
	}

	execName, err := b.job.StartJobExecution(ctx, aca.JobRequest{
		Image:   b.CDImage,
		Command: []string{"/app/cd", verb, payload},
		Envs:    envMap,
		Timeout: 30 * time.Minute,
	})
	if err != nil {
		return nil, err
	}
	b.cdRunID = execName
	b.cdEtag = etag
	return &client.DeployResponse{
		CdId:   execName,
		CdType: defangv1.CdType_CD_TYPE_AZURE_ACI_JOBID,
		DeployResponse: &defangv1.DeployResponse{
			Etag: etag, Services: serviceInfos,
		},
	}, nil
}

// GetDeploymentStatus implements client.Provider. CD container output is streamed
// live via QueryLogs (follow=true) rather than drained from Log Analytics here,
// so this method can return as soon as the job reaches a terminal state.
func (b *ByocAzure) GetDeploymentStatus(ctx context.Context) (bool, error) {
	if b.cdRunID == "" {
		return false, nil
	}
	status, err := b.job.GetJobExecutionStatus(ctx, b.cdRunID)
	if err != nil {
		// Return the raw error so WaitForCdTaskExit's isTransientError can
		// retry on flaky failures (e.g. AzureCLICredential subprocess timeouts).
		// Wrapping as ErrDeploymentFailed here would mask transient errors as
		// permanent deployment failures.
		return false, err
	}
	if !status.IsTerminal() {
		return false, nil
	}
	if !status.IsSuccess() {
		msg := string(status.Status)
		if status.ErrorMessage != "" {
			msg += ": " + status.ErrorMessage
		}
		return true, client.ErrDeploymentFailed{Message: fmt.Sprintf("CD job %s: %s", b.cdRunID, msg)}
	}
	return true, nil
}

// GetPrivateDomain implements byoc.ProjectBackend.
func (b *ByocAzure) GetPrivateDomain(projectName string) string {
	return b.GetProjectLabel(projectName) + ".internal"
}

// GetProjectUpdate implements byoc.ProjectBackend. It is read-only — it does
// not create the CD resource group, storage account, container apps
// environment, or any other provisioning side effect. On a subscription
// where defang has never been deployed the storage account lookup returns
// nothing and we report client.ErrNotExist immediately.
//
// The blob lives in the dedicated `projects` container (populated by the CD
// task before each deploy) at key `{project}/{stack}/project.pb`.
func (b *ByocAzure) GetProjectUpdate(ctx context.Context, projectName string) (*defangv1.ProjectUpdate, error) {
	if projectName == "" {
		return nil, client.ErrNotExist
	}
	if err := b.setUpLocation(); err != nil {
		return nil, err
	}
	storageAccount, err := b.driver.FindStorageAccount(ctx)
	if err != nil {
		return nil, err
	}
	if storageAccount == "" {
		// CD storage account hasn't been provisioned yet.
		return nil, client.ErrNotExist
	}

	// GetProjectUpdatePath returns "projects/{project}/{stack}/project.pb".
	// The `projects` container already provides the top-level namespace, so
	// strip the leading "projects/" when addressing the blob.
	key := strings.TrimPrefix(b.GetProjectUpdatePath(projectName), "projects/")
	term.Debug("Getting project update from blob:", cd.ProjectsContainerName, key)
	pbBytes, err := b.driver.DownloadBlobFromContainer(ctx, cd.ProjectsContainerName, key)
	if err != nil {
		var respErr *azcore.ResponseError
		if errors.As(err, &respErr) && (respErr.StatusCode == 404 || respErr.ErrorCode == "ContainerNotFound" || respErr.ErrorCode == "BlobNotFound") {
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

// GetService implements client.Provider by fetching GetServices and filtering
// to the requested name — same pattern as the AWS and GCP providers.
func (b *ByocAzure) GetService(ctx context.Context, req *defangv1.GetRequest) (*defangv1.ServiceInfo, error) {
	all, err := b.GetServices(ctx, &defangv1.GetServicesRequest{Project: req.Project})
	if err != nil {
		return nil, err
	}
	for _, service := range all.Services {
		if service.Service.Name == req.Name {
			return service, nil
		}
	}
	return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("service %q not found", req.Name))
}

// GetServices implements client.Provider by reading the ProjectUpdate blob
// that the CD task uploads during Deploy — same pattern as the AWS and GCP
// providers.
func (b *ByocAzure) GetServices(ctx context.Context, req *defangv1.GetServicesRequest) (*defangv1.GetServicesResponse, error) {
	projUpdate, err := b.GetProjectUpdate(ctx, req.Project)
	if err != nil {
		if errors.Is(err, client.ErrNotExist) {
			return &defangv1.GetServicesResponse{}, nil
		}
		return nil, err
	}
	return &defangv1.GetServicesResponse{
		Services: projUpdate.Services,
		Project:  projUpdate.Project,
	}, nil
}

// ListConfig implements client.Provider. Read-only: when the project's
// App Configuration store or Key Vault hasn't been provisioned yet, returns
// an empty list instead of creating them.
func (b *ByocAzure) ListConfig(ctx context.Context, req *defangv1.ListConfigsRequest) (*defangv1.Secrets, error) {
	found, err := b.findForConfig(ctx, req.Project)
	if err != nil {
		return nil, err
	}
	if !found {
		return &defangv1.Secrets{}, nil // nothing configured yet
	}
	prefix := b.StackDir(req.Project, "")
	secretPrefix := keyvault.ToSecretName(prefix)
	term.Debugf("Listing Key Vault secrets with prefix %q (sanitized: %q)", prefix, secretPrefix)
	entries, err := b.kv.ListSecrets(ctx, secretPrefix)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.OriginalKey == "" || !strings.HasPrefix(e.OriginalKey, prefix) {
			continue
		}
		names = append(names, strings.TrimPrefix(e.OriginalKey, prefix))
	}
	return &defangv1.Secrets{Names: names}, nil
}

// PrepareDomainDelegation implements client.Provider.
func (b *ByocAzure) PrepareDomainDelegation(context.Context, client.PrepareDomainDelegationRequest) (*client.PrepareDomainDelegationResponse, error) {
	return nil, nil // TODO: implement domain delegation for Azure
}

// Preview implements client.Provider.
func (b *ByocAzure) Preview(ctx context.Context, req *client.DeployRequest) (*client.DeployResponse, error) {
	return b.deploy(ctx, req, "preview")
}

// PutConfig implements client.Provider.
func (b *ByocAzure) PutConfig(ctx context.Context, req *defangv1.PutConfigRequest) error {
	if err := b.setUpForConfig(ctx, req.Project); err != nil {
		return err
	}
	key := b.StackDir(req.Project, req.Name)
	secretName := keyvault.ToSecretName(key)
	term.Debugf("Putting Key Vault secret %q (original key %q)", secretName, key)
	if err := b.kv.PutSecret(ctx, secretName, req.Value, key); err != nil {
		return fmt.Errorf("failed to put Key Vault secret: %w", err)
	}
	return nil
}

// QueryLogs implements client.Provider.
// Only CD container logs are supported; service logs are not yet implemented.
func (b *ByocAzure) QueryLogs(ctx context.Context, req *defangv1.TailRequest) (iter.Seq2[*defangv1.TailResponse, error], error) {
	// Match the request etag to the stored CD etag so we tail the correct run.
	if b.cdRunID == "" || (req.Etag != "" && req.Etag != b.cdEtag) {
		return nil, fmt.Errorf("QueryLogs: no matching CD deployment for etag %q", req.Etag)
	}

	const cdServiceName = "defang-cd"
	etag := b.cdEtag

	if req.Follow {
		logIter, err := b.job.TailJobLogs(ctx, b.cdRunID)
		if err != nil {
			return nil, err
		}

		// Run ACR log iterator in a goroutine so we can select over it and ACA logs.
		type cdLogEntry struct {
			line string
			err  error
		}
		cdCh := make(chan cdLogEntry)
		go func() {
			defer close(cdCh)
			for line, err := range logIter {
				select {
				case cdCh <- cdLogEntry{line: line, err: err}:
				case <-ctx.Done():
					return
				}
			}
		}()

		projectRG := b.projectResourceGroupName(req.Project)

		// Watch Container App logs from the PROJECT resource group.
		acaClient := &aca.ContainerApp{
			Azure:         b.driver.Azure,
			ResourceGroup: projectRG,
		}
		acaCh := acaClient.WatchLogs(ctx)

		// Watch ACR build logs from the PROJECT resource group.
		buildWatcher := &acr.BuildLogWatcher{
			Azure:         b.driver.Azure,
			ResourceGroup: projectRG,
		}
		buildCh := buildWatcher.WatchBuildLogs(ctx)

		return func(yield func(*defangv1.TailResponse, error) bool) {
			for {
				select {
				case entry, ok := <-cdCh:
					if !ok {
						cdCh = nil
						continue
					}
					if entry.err != nil {
						if !yield(nil, entry.err) {
							return
						}
						continue
					}
					if !yield(&defangv1.TailResponse{
						Entries: []*defangv1.LogEntry{{
							Message:   entry.line,
							Service:   cdServiceName,
							Etag:      etag,
							Timestamp: timestamppb.Now(),
						}},
						Service: cdServiceName,
						Etag:    etag,
					}, nil) {
						return
					}
				case svc, ok := <-acaCh:
					if !ok {
						acaCh = nil
						continue
					}
					if svc.Err != nil {
						term.Debugf("Container Apps log error for %q: %v", svc.AppName, svc.Err)
						continue
					}
					if !yield(&defangv1.TailResponse{
						Entries: []*defangv1.LogEntry{{
							Message:   svc.Message,
							Service:   svc.AppName,
							Etag:      etag,
							Timestamp: timestamppb.Now(),
						}},
						Service: svc.AppName,
						Etag:    etag,
					}, nil) {
						return
					}
				case build, ok := <-buildCh:
					if !ok {
						buildCh = nil
						continue
					}
					if build.Err != nil {
						term.Debugf("ACR build log error for %q: %v", build.Service, build.Err)
						continue
					}
					if !yield(&defangv1.TailResponse{
						Entries: []*defangv1.LogEntry{{
							Message:   build.Line,
							Service:   build.Service + "-build",
							Etag:      etag,
							Stderr:    true, // show build logs even when not verbose
							Timestamp: timestamppb.Now(),
						}},
						Service: build.Service + "-build",
						Etag:    etag,
					}, nil) {
						return
					}
				case <-ctx.Done():
					return
				}
				if cdCh == nil && acaCh == nil && buildCh == nil {
					return
				}
			}
		}, nil
	}

	// Non-follow: return a snapshot of current log content.
	content, err := b.job.ReadJobLogs(ctx, b.cdRunID)
	if err != nil {
		return nil, err
	}
	return func(yield func(*defangv1.TailResponse, error) bool) {
		if content == "" {
			return
		}
		yield(&defangv1.TailResponse{
			Entries: []*defangv1.LogEntry{{
				Message:   content,
				Service:   cdServiceName,
				Etag:      etag,
				Timestamp: timestamppb.Now(),
			}},
			Service: cdServiceName,
			Etag:    etag,
		}, nil)
	}, nil
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
	return func(yield func(*defangv1.SubscribeResponse, error) bool) {
		// TODO: Implement subscription to deployment events for Azure
	}, nil
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
