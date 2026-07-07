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
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc/state"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	cloudazure "github.com/DefangLabs/defang/src/pkg/clouds/azure"
	"github.com/DefangLabs/defang/src/pkg/clouds/azure/aca"
	"github.com/DefangLabs/defang/src/pkg/clouds/azure/acr"
	"github.com/DefangLabs/defang/src/pkg/clouds/azure/cd"
	azuredns "github.com/DefangLabs/defang/src/pkg/clouds/azure/dns"
	"github.com/DefangLabs/defang/src/pkg/clouds/azure/keyvault"
	defanghttp "github.com/DefangLabs/defang/src/pkg/http"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/tokenstore"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	composeTypes "github.com/compose-spec/compose-go/v2/types"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type ByocAzure struct {
	*byoc.ByocBaseClient

	driver  *cd.Driver
	job     *aca.Job
	kv      *keyvault.KeyVault
	cdRunID string
	cdEtag  string
	cdStart time.Time

	// delegateDomainZone contains the delegate domain whose public DNS zone
	// PrepareDomainDelegation created, mirroring the GCP provider's
	// ByocGcp.delegateDomainZone field. Currently informational only — set on
	// success, not yet read elsewhere. The GCP provider follows the same
	// pattern: the destroy path relies on the project resource group teardown
	// to remove the zone rather than tracking it explicitly. Kept here as a
	// hook for a future cleanup path that needs to know which zone to delete.
	delegateDomainZone string
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

// CdCommand implements byoc.ProjectBackend.
func (b *ByocAzure) CdCommand(ctx context.Context, req client.CdCommandRequest) (*client.CdCommandResponse, error) {
	defer term.Timing()()
	if err := b.setUpJob(ctx); err != nil {
		return nil, err
	}

	etag := types.NewEtag()
	cdStart := time.Now()
	execName, err := b.runCdCommand(ctx, cdCommand{
		command:   []string{string(req.Command)},
		etag:      etag,
		project:   req.Project,
		statesUrl: req.StatesUrl,
		eventsUrl: req.EventsUrl,
	})
	if err != nil {
		return nil, err
	}
	b.cdRunID = execName
	b.cdEtag = etag
	b.cdStart = cdStart
	return &client.CdCommandResponse{
		CdId:   execName,
		CdType: defangv1.CdType_CD_TYPE_AZURE_ACA_JOBID,
		ETag:   etag,
	}, nil
}

// CdList implements byoc.ProjectBackend. Read-only: when the CD storage
// account hasn't been provisioned yet (fresh subscription), returns an empty
// iterator instead of bootstrapping the resource group / storage account.
func (b *ByocAzure) CdList(ctx context.Context, _ bool) (iter.Seq[state.Info], error) {
	defer term.Timing()()
	if err := b.setUpLocation(); err != nil {
		return nil, err
	}
	storageAccount, err := b.driver.FindStorageAccount(ctx)
	if err != nil {
		return nil, err
	}
	if storageAccount == "" {
		return func(yield func(state.Info) bool) {}, nil
	}

	blobs, err := b.driver.IterateBlobs(ctx, ".pulumi/stacks/")
	if err != nil {
		return nil, err
	}

	term.Debug("Iterating blobs in container to find Pulumi state files")
	return func(yield func(state.Info) bool) {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		stackCh := make(chan state.Info)

		// Spawn one download+parse goroutine per blob, capped by SetLimit.
		// Run from a goroutine so the consumer loop can drain stackCh
		// concurrently — otherwise workers block sending to the unbuffered
		// stackCh and g.Go blocks at the limit.
		const maxDownloaders = 4 // not CPU bound so unrelated to runtime.GOMAXPROCS
		g, gctx := errgroup.WithContext(ctx)
		g.SetLimit(maxDownloaders)
		go func() {
			defer close(stackCh)
			for item, err := range blobs {
				if err != nil {
					term.Debugf("Error iterating blobs: %v", err)
					break
				}
				if gctx.Err() != nil {
					break
				}
				g.Go(func() error {
					st, err := state.ParsePulumiStateFile(gctx, item, func(ctx context.Context, blobName string) ([]byte, error) {
						return b.driver.DownloadBlob(ctx, blobName) // slow
					})
					if err != nil {
						term.Debugf("Skipping %q: %v", item.Name(), err)
						return nil
					}
					if st == nil {
						return nil
					}
					select {
					case <-gctx.Done():
						return gctx.Err()
					case stackCh <- state.Info{
						Project:   st.Project,
						Stack:     st.Name,
						Workspace: string(st.Workspace),
						CdRegion:  b.driver.Location.String(),
					}:
					}
					return nil
				})
			}
			g.Wait()
		}()

		for stack := range stackCh {
			if !yield(stack) {
				return // deferred cancel() unblocks workers and producer
			}
		}
	}, nil
}

// AccountInfo implements client.Provider.
func (b *ByocAzure) AccountInfo(context.Context) (*client.AccountInfo, error) {
	if err := b.setUpLocation(); err != nil {
		return nil, fmt.Errorf("AccountInfo: %w", err)
	}
	return &client.AccountInfo{
		AccountID: b.driver.SubscriptionID,
		Provider:  client.ProviderAzure,
		Region:    b.driver.Location.String(),
	}, nil
}

// CreateUploadURL implements client.Provider.
func (b *ByocAzure) CreateUploadURL(ctx context.Context, req *defangv1.UploadURLRequest) (*defangv1.UploadURLResponse, error) {
	defer term.Timing()()
	if err := b.SetUpCD(ctx, false); err != nil {
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
	defer term.Timing()()
	return nil, fmt.Errorf("Delete: %w", errors.ErrUnsupported)
}

// DeleteConfig implements client.Provider. Read-only for discovery: if the
// Key Vault doesn't exist yet, there's nothing to delete — return success
// instead of provisioning it just to tear down.
func (b *ByocAzure) DeleteConfig(ctx context.Context, secrets *defangv1.Secrets) error {
	defer term.Timing()()
	found, err := b.findForConfig(ctx, secrets.Project)
	if err != nil {
		return err
	}
	if !found {
		return nil // nothing configured yet, nothing to delete
	}
	for _, name := range secrets.Names {
		secretName := keyvault.ToSecretName(name)
		term.Debugf("Deleting Key Vault secret %q", secretName)
		if err := b.kv.DeleteSecret(ctx, secretName); err != nil {
			return fmt.Errorf("failed to delete Key Vault secret %q: %w", name, err)
		}
	}
	return nil
}

// setUpLocation lazily resolves AZURE_LOCATION (deploy target) and
// AZURE_SUBSCRIPTION_ID from the environment and syncs the values to the
// job. It makes no API calls. Job.Location is set tentatively to the deploy
// target; SetUpCD overrides it to the CD primary region (cdLocation) once
// the shared RG has been resolved.
func (b *ByocAzure) setUpLocation() error {
	term.Debug("setUpLocation: resolving AZURE_LOCATION and AZURE_SUBSCRIPTION_ID")
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

// syncJobToCdLocation overrides Job.Location with the resolved CD primary
// region. Must be called after b.driver.SetUpResourceGroup so cdLocation is
// populated; otherwise a no-op. The Job's RG, log workspace, managed env,
// and CD container all live in cdLocation, not the deploy target.
func (b *ByocAzure) syncJobToCdLocation() {
	if cd := b.driver.CdLocation(); cd != "" {
		b.job.Location = cd
	}
}

// projectResourceGroupName returns the resource group name for project-specific resources
// (Key Vault config store and deployed services). It must match what the Pulumi Azure
// program creates: {Prefix}-{project}-{stack}, e.g. "Defang-portal-production" for the
// bare project name "portal" on stack "production". This group is separate from the
// shared CD resource group.
func (b *ByocAzure) projectResourceGroupName(projectName string) string {
	return b.Prefix + "-" + projectName + "-" + b.PulumiStack
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
	defer term.Timing()()
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
	defer term.Timing()()
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

// SetUpCD sets up the shared CD infrastructure: resource group, blob storage, the Container
// Apps environment, and the job's managed identity. It does NOT create the CD job itself
// (SetUpJob must be called separately with env vars baked in) and does NOT set up
// project-specific resources (use setUpForConfig for App Configuration).
func (b *ByocAzure) SetUpCD(ctx context.Context, force bool) error {
	defer term.Timing()()
	if err := b.setUpLocation(); err != nil {
		return err
	}

	if b.SetupDone && !force {
		return nil
	}

	// Create or adopt the shared CD resource group ("defang-cd"). On first
	// deploy this creates the RG in b.driver.Location; on later deploys to
	// any region it adopts the existing RG and resolves cdLocation from it.
	if err := b.driver.SetUpResourceGroup(ctx); err != nil {
		return err
	}
	// Now that cdLocation is known, the Job's resources (workspace, env,
	// container) belong in the CD region, not the per-deploy target.
	b.syncJobToCdLocation()

	if _, err := b.driver.SetUpStorageAccount(ctx); err != nil {
		return fmt.Errorf("failed to set up storage account: %w", err)
	}

	b.SetupDone = true
	return nil
}

// setUpJob creates/updates the CD job with the given env vars baked into its template,
// and grants the job's managed identity read access to the CD storage account. SetUpCD
// runs first so the shared CD region (cdLocation) is resolved before any per-region
// resources (workspace, environment, job) are created — those must live in cdLocation,
// not the deploy target. The CD image is pulled anonymously — its registry must allow
// anonymous pull.
func (b *ByocAzure) setUpJob(ctx context.Context) error {
	defer term.Timing()()
	if err := b.SetUpCD(ctx, false); err != nil {
		return err
	}
	if err := b.job.SetUpManagedEnvironment(ctx); err != nil {
		return fmt.Errorf("failed to set up container apps environment: %w", err)
	}
	if b.CDImage == "" {
		return errors.New("CD image is not set; please set the DEFANG_CD_IMAGE environment variable")
	}
	if err := b.job.SetUpJob(ctx, b.CDImage, nil); err != nil {
		return fmt.Errorf("failed to set up CD job: %w", err)
	}
	if err := b.job.SetUpManagedIdentity(ctx, b.driver.StorageAccount); err != nil {
		return fmt.Errorf("failed to set up managed identity: %w", err)
	}
	return nil
}

// environment returns the environment map that every CD container run needs.
func (b *ByocAzure) environment(projectName string) (map[string]string, error) {
	// Pulumi state lives in its own container (`pulumi`), separate from the
	// `uploads` container (etag payloads, tarballs) and the `projects`
	// container (project.pb audit blobs written by the CD task).
	defangStateUrl := fmt.Sprintf(`azblob://%s?storage_account=%s`, b.driver.BlobContainerName, b.driver.StorageAccount)
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
	defer term.Timing()()
	return b.deploy(ctx, req, "up")
}

type cdCommand struct {
	command        []string
	etag           types.ETag
	mode           defangv1.DeploymentMode
	project        string
	statesUrl      string
	eventsUrl      string
	delegateDomain string // forwarded to CD as DOMAIN; empty when deploying without delegation
}

func (b *ByocAzure) runCdCommand(ctx context.Context, cmd cdCommand) (string, error) {
	defer term.Timing()()
	// Setup the deployment environment
	env, err := b.environment(cmd.project)
	if err != nil {
		return "", err
	}
	if cmd.etag != "" {
		env[aca.EnvVarEtag] = cmd.etag
	}
	env["DEFANG_MODE"] = strings.ToLower(cmd.mode.String())

	if cmd.statesUrl != "" {
		env["DEFANG_STATES_UPLOAD_URL"] = cmd.statesUrl
	}

	if cmd.eventsUrl != "" {
		env["DEFANG_EVENTS_UPLOAD_URL"] = cmd.eventsUrl
	}

	// DOMAIN is the project delegate domain (e.g. "myproj-stack.tenant.defang.app");
	// the CD program reads it and sets defang:domain Pulumi config, which the
	// Azure inline program forwards to ProjectInputs.Domain so per-service
	// DNS records and managed certs land in the right zone. Mirrors AWS.
	if cmd.delegateDomain != "" {
		env["DOMAIN"] = cmd.delegateDomain
	}

	if os.Getenv("DEFANG_PULUMI_DIR") != "" {
		// Run the cd binary locally from $DEFANG_PULUMI_DIR/cd instead of
		// starting it as a Container Apps Job. Useful for iterating on cd
		// code without rebuilding/pushing the cd image. Authentication uses
		// the host's az login chain (DefaultAzureCredential).
		debugEnv := []string{
			"AZURE_LOCATION=" + b.driver.Location.String(),
			"AZURE_SUBSCRIPTION_ID=" + b.driver.SubscriptionID,
		}
		for k, v := range env {
			debugEnv = append(debugEnv, k+"="+v)
		}
		if err := byoc.DebugPulumiCD(ctx, debugEnv, cmd.command...); err != nil {
			return "", err
		}
	}

	return b.job.StartJobExecution(ctx, aca.JobRequest{
		Image:   b.CDImage,
		Command: append([]string{"/app/cd"}, cmd.command...),
		Envs:    env,
		Timeout: 30 * time.Minute,
	})
}

func (b *ByocAzure) deploy(ctx context.Context, req *client.DeployRequest, verb string) (*client.DeployResponse, error) {
	defer term.Timing()()
	if b.CDImage == "" {
		return nil, errors.New("CD image is not set; please set the DEFANG_CD_IMAGE environment variable")
	}

	// If multiple Compose files were provided, req.Compose is the merged representation of all the files
	project, err := compose.LoadFromContent(ctx, req.Compose, "")
	if err != nil {
		return nil, err
	}

	etag := types.NewEtag()
	serviceInfos, err := b.GetServiceInfos(ctx, project.Name, req.DelegateDomain, etag, project.Services)
	if err != nil {
		return nil, err
	}

	data, err := proto.Marshal(&defangv1.ProjectUpdate{
		CdVersion:     b.CDImage,
		Compose:       req.Compose,
		Etag:          etag,
		Mode:          req.Mode,
		PulumiVersion: b.PulumiVersion,
		Services:      serviceInfos,
		Recipe:        req.Recipe,
	})
	if err != nil {
		return nil, err
	}

	if err := b.setUpJob(ctx); err != nil {
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

	cdStart := time.Now()
	execName, err := b.runCdCommand(ctx, cdCommand{
		command:        []string{verb, payload},
		etag:           etag,
		mode:           req.Mode,
		project:        project.Name,
		statesUrl:      req.StatesUrl,
		eventsUrl:      req.EventsUrl,
		delegateDomain: req.DelegateDomain,
	})
	if err != nil {
		return nil, err
	}
	b.cdRunID = execName
	b.cdEtag = etag
	b.cdStart = cdStart
	return &client.DeployResponse{
		CdId:   execName,
		CdType: defangv1.CdType_CD_TYPE_AZURE_ACA_JOBID,
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
	defer term.Timing()()
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

	key := b.GetProjectUpdatePath(projectName)
	term.Debug("Getting project update from blob:", b.driver.BlobContainerName, key)
	pbBytes, err := b.driver.DownloadBlob(ctx, key)
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
	defer term.Timing()()
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
	defer term.Timing()()
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
// Key Vault hasn't been provisioned yet, returns an empty list instead of
// creating it. The vault is per-project-stack, so every secret in it belongs
// to this project — no prefix filtering is needed.
func (b *ByocAzure) ListConfig(ctx context.Context, req *defangv1.ListConfigsRequest) (*defangv1.Secrets, error) {
	defer term.Timing()()
	found, err := b.findForConfig(ctx, req.Project)
	if err != nil {
		return nil, err
	}
	if !found {
		return &defangv1.Secrets{}, nil // nothing configured yet
	}
	entries, err := b.kv.ListSecrets(ctx, "")
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		// Prefer the defang-config tag (preserves underscores) but fall
		// back to the secret name if the tag is missing for any reason.
		if e.OriginalKey != "" {
			names = append(names, e.OriginalKey)
		} else {
			names = append(names, e.Name)
		}
	}
	return &defangv1.Secrets{Names: names}, nil
}

// PrepareDomainDelegation implements client.Provider. It creates a public
// Azure DNS zone for the delegate domain and returns the zone's authoritative
// name servers so Fabric can point NS records at it, mirroring the GCP
// provider. The zone lives in the per-project-stack resource group so it is
// owned by this deployment and torn down with it. Azure has no equivalent of
// Route53's reusable delegation sets, so DelegationSetId is left empty (as
// with GCP).
func (b *ByocAzure) PrepareDomainDelegation(ctx context.Context, req client.PrepareDomainDelegationRequest) (*client.PrepareDomainDelegationResponse, error) {
	defer term.Timing()()
	if req.DelegateDomain == "" {
		return nil, nil // no delegate domain configured; nothing to delegate
	}
	if err := b.setUpLocation(); err != nil {
		return nil, err
	}
	term.Debugf("Preparing domain delegation for %s", req.DelegateDomain)

	// The zone needs a home before it can be created. On a fresh deploy Pulumi
	// has not created the project resource group yet, so ensure it exists
	// (idempotent CreateOrUpdate) the same way config setup does.
	rgName := b.projectResourceGroupName(req.Project)
	if err := b.driver.CreateResourceGroup(ctx, rgName); err != nil {
		return nil, err
	}

	nameServers, err := azuredns.New(rgName, b.driver.Azure).EnsureZoneExists(ctx, req.DelegateDomain)
	if err != nil {
		return nil, err
	}
	b.delegateDomainZone = req.DelegateDomain
	term.Debugf("Zone %s ready with nameservers %v", req.DelegateDomain, nameServers)
	return &client.PrepareDomainDelegationResponse{
		NameServers: nameServers,
	}, nil
}

// HasDelegatedSubdomain implements client.Provider. Azure delegates a
// subdomain zone during deploy (PrepareDomainDelegation +
// CreateDelegateSubdomainZone), so the matching DeleteSubdomainZone must run
// on destroy.
func (*ByocAzure) HasDelegatedSubdomain() bool {
	return true
}

// Preview implements client.Provider.
func (b *ByocAzure) Preview(ctx context.Context, req *client.DeployRequest) (*client.DeployResponse, error) {
	defer term.Timing()()
	return b.deploy(ctx, req, "preview")
}

// PutConfig implements client.Provider.
func (b *ByocAzure) PutConfig(ctx context.Context, req *defangv1.PutConfigRequest) error {
	defer term.Timing()()
	if err := b.setUpForConfig(ctx, req.Project); err != nil {
		return err
	}
	secretName := keyvault.ToSecretName(req.Name)
	term.Debugf("Putting Key Vault secret %q (original key %q)", secretName, req.Name)
	if err := b.kv.PutSecret(ctx, secretName, req.Value, req.Name); err != nil {
		return fmt.Errorf("failed to put Key Vault secret: %w", err)
	}
	return nil
}

// QueryLogs implements client.Provider. It merges three log sources for the project:
// CD (deployment) logs from the Container Apps Job, service logs from the project's
// Container Apps, and build logs from ACR. When req.Follow is set each source streams
// live; otherwise each returns a one-shot snapshot and the iterator ends once all are
// drained.
func (b *ByocAzure) QueryLogs(ctx context.Context, req *defangv1.TailRequest) (iter.Seq2[*defangv1.TailResponse, error], error) {
	const cdServiceName = "defang-cd"

	// setUpLocation resolves AZURE_LOCATION/AZURE_SUBSCRIPTION_ID onto the driver and
	// job (no API calls). The service and build watchers below use b.driver.Azure, so
	// this must run even when there's no etag to look up a CD run by.
	if err := b.setUpLocation(); err != nil {
		return nil, err
	}

	// Resolve the CD job execution for this request. The deploying process caches
	// the run ID in memory; a standalone `defang logs` has none, so recover it by
	// matching the request etag against each execution's recorded ETAG env var.
	runID := b.cdRunID
	etag := b.cdEtag
	if req.Etag != "" && req.Etag != b.cdEtag {
		runID, etag = "", req.Etag
	}
	if runID == "" && etag != "" {
		found, err := b.job.FindExecutionByEtag(ctx, etag)
		if err != nil {
			return nil, fmt.Errorf("failed to find CD deployment for etag %q: %w", etag, err)
		}
		runID = found
	}
	if runID == "" {
		// Unknown or empty etag: no CD run to tail. Service and build logs are keyed
		// by resource group rather than the CD execution, so still surface those.
		term.Warnf("No CD logs found for etag %q; showing service and build logs only", req.Etag)
	}

	// CD logs. cdCh stays nil when there's no CD run, which makes the select below skip
	// it and treat the CD source as already drained. Follow streams line-by-line; the
	// snapshot reads the whole buffered run content once.
	type cdLogEntry struct {
		line string
		err  error
	}
	var cdCh chan cdLogEntry
	if runID != "" {
		cdCh = make(chan cdLogEntry)
		if req.Follow {
			logIter, err := b.job.TailJobLogs(ctx, runID)
			if err != nil {
				return nil, err
			}
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
		} else {
			content, err := b.job.ReadJobLogs(ctx, runID)
			if err != nil {
				return nil, err
			}
			go func() {
				defer close(cdCh)
				if content == "" {
					return
				}
				select {
				case cdCh <- cdLogEntry{line: content}:
				case <-ctx.Done():
				}
			}()
		}
	}

	projectRG := b.projectResourceGroupName(req.Project)

	// Service logs from the project's Container Apps and build logs from ACR both
	// live in the PROJECT resource group (independent of the CD run). For a one-shot
	// query, if that group doesn't exist the project isn't deployed under this
	// name/stack — surface one clear message instead of letting both watchers fail
	// with raw "ResourceGroupNotFound" errors. In follow mode we skip the check: the
	// group may be created mid-session (deploy-then-tail), so the watchers poll for it.
	var acaCh <-chan aca.ServiceLogEntry
	var buildCh <-chan acr.BuildLogEntry
	startWatchers := true
	if !req.Follow {
		exists, err := b.driver.ResourceGroupExists(ctx, projectRG)
		if err != nil {
			return nil, err
		}
		if !exists {
			term.Warnf("No deployed services found for project %q on stack %q (resource group %q not found)", req.Project, b.PulumiStack, projectRG)
			startWatchers = false
		}
	}
	if startWatchers {
		acaClient := &aca.ContainerApp{
			Azure:         b.driver.Azure,
			ResourceGroup: projectRG,
		}
		acaCh = acaClient.WatchLogs(ctx, req.Follow)

		buildWatcher := &acr.BuildLogWatcher{
			Azure:         b.driver.Azure,
			ResourceGroup: projectRG,
		}
		buildCh = buildWatcher.WatchBuildLogs(ctx, req.Follow)
	}

	return func(yield func(*defangv1.TailResponse, error) bool) {
		for {
			// Check for completion at the top: a closed source is set to nil below, and
			// selecting over all-nil channels would block forever (the non-follow case
			// where every source drains). In follow mode the channels stay open until
			// ctx is cancelled.
			if cdCh == nil && acaCh == nil && buildCh == nil {
				return
			}
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
					// Non-fatal: warn (so the failure isn't silently swallowed) but keep
					// reading the other sources rather than ending the whole tail.
					term.Debugf("Could not read service logs for %q: %v", svc.AppName, svc.Err)
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
					// Non-fatal: warn but keep reading the other sources (see above).
					term.Warnf("Could not read build logs for %q: %v", build.Service, build.Err)
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
		}
	}, nil
}

// RemoteProjectName implements client.Provider.
// Subtle: this method shadows the method (*ByocBaseClient).RemoteProjectName of ByocAzure.ByocBaseClient.
func (b *ByocAzure) RemoteProjectName(context.Context) (string, error) {
	defer term.Timing()()
	return "", fmt.Errorf("RemoteProjectName: %w", errors.ErrUnsupported)
}

// ServiceDNS implements client.Provider.
// Subtle: this method shadows the method (*ByocBaseClient).ServiceDNS of ByocAzure.ByocBaseClient.
func (b *ByocAzure) ServiceDNS(host string) string {
	defer term.Timing()()
	return host
}

// TearDown implements client.Provider.
func (b *ByocAzure) TearDown(ctx context.Context) error {
	defer term.Timing()()
	return b.driver.TearDown(ctx)
}

// TearDownCD implements client.Provider.
func (b *ByocAzure) TearDownCD(context.Context) error {
	defer term.Timing()()
	return fmt.Errorf("TearDownCD: %w", errors.ErrUnsupported)
}

// UpdateShardDomain implements client.DNSResolver.
func (b *ByocAzure) UpdateShardDomain(context.Context) error {
	return fmt.Errorf("UpdateShardDomain: %w", errors.ErrUnsupported)
}

// UpdateServiceInfo implements byoc.ServiceInfoUpdater. When a service has a
// `domainname` set in compose, mark it for managed-cert issuance so
// `defang cert generate` picks it up via the CertIssuer path. Azure Container
// Apps managed certs are free, auto-renewing, and validated via CNAME — no
// hosted-zone presence required (unlike AWS, where ZoneId triggers a different
// path).
func (b *ByocAzure) UpdateServiceInfo(_ context.Context, si *defangv1.ServiceInfo, _, _ string, service composeTypes.ServiceConfig) error {
	if service.DomainName == "" {
		return nil
	}
	si.UseAcmeCert = true
	return nil
}
