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
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc/state"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	cloudazure "github.com/DefangLabs/defang/src/pkg/clouds/azure"
	"github.com/DefangLabs/defang/src/pkg/clouds/azure/aca"
	"github.com/DefangLabs/defang/src/pkg/clouds/azure/acr"
	"github.com/DefangLabs/defang/src/pkg/clouds/azure/appcfg"
	"github.com/DefangLabs/defang/src/pkg/clouds/azure/cd"
	"github.com/DefangLabs/defang/src/pkg/clouds/azure/keyvault"
	defanghttp "github.com/DefangLabs/defang/src/pkg/http"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type ByocAzure struct {
	*byoc.ByocBaseClient

	driver        *cd.Driver
	job           *aca.Job
	appCfg        *appcfg.AppConfiguration
	kv            *keyvault.KeyVault
	cdRunID       string
	cdEtag        string
	cdLogsDrained bool // true once we've drained CD logs from Log Analytics for cdRunID
	setUpDone     bool // true once full setUp has completed; prevents redundant API calls
}

var _ client.Provider = (*ByocAzure)(nil)

func NewByocProvider(ctx context.Context, tenantLabel types.TenantLabel, stack string) *ByocAzure {
	b := &ByocAzure{
		driver: cd.New("defang-cd", ""), // default location => from AZURE_LOCATION env var
		job:    &aca.Job{},
	}
	b.ByocBaseClient = byoc.NewByocBaseClient(tenantLabel, b, stack)
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
	b.cdLogsDrained = false
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
func (b *ByocAzure) DeleteConfig(ctx context.Context, secrets *defangv1.Secrets) error {
	if err := b.setUpForConfig(ctx, secrets.Project); err != nil {
		return err
	}
	for _, name := range secrets.Names {
		key := b.StackDir(secrets.Project, name)
		secretName := keyvault.ToSecretName(key)
		term.Debugf("Deleting Key Vault secret %q", secretName)
		if err := b.kv.DeleteSecret(ctx, secretName); err != nil {
			return fmt.Errorf("failed to delete Key Vault secret %q: %w", name, err)
		}
		term.Debugf("Deleting App Configuration key %q", key)
		if err := b.appCfg.DeleteSetting(ctx, key); err != nil {
			return fmt.Errorf("failed to delete config %q: %w", name, err)
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
	return "defang-" + projectName + "-" + b.PulumiStack + "-" + b.driver.Location.String()
}

// setUpForConfig sets up the project-specific App Configuration store and Key Vault.
// It creates (or reuses) a resource group named {project}-{stack}-{location}, distinct from
// the shared CD resource group, so each project deployment has its own parameter store.
func (b *ByocAzure) setUpForConfig(ctx context.Context, projectName string) error {
	if err := b.setUpLocation(); err != nil {
		return err
	}
	rgName := b.projectResourceGroupName(projectName)
	if err := b.driver.CreateResourceGroup(ctx, rgName); err != nil {
		return err
	}
	if b.appCfg == nil {
		b.appCfg = appcfg.New(rgName, b.driver.Location, b.driver.SubscriptionID)
		if err := b.appCfg.SetUp(ctx); err != nil {
			return err
		}
	}
	if b.kv == nil {
		b.kv = keyvault.New(rgName, b.driver.Location, b.driver.SubscriptionID)
		if err := b.kv.SetUp(ctx); err != nil {
			return err
		}
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
	defangStateUrl := fmt.Sprintf(`azblob://%s?storage_account=%s`, b.driver.BlobContainerName, b.driver.StorageAccount)
	pulumiBackendKey, pulumiBackendValue, err := byoc.GetPulumiBackend(defangStateUrl)
	if err != nil {
		return nil, err
	}
	env := map[string]string{
		"AZURE_LOCATION":             b.driver.Location.String(),
		"AZURE_RESOURCE_GROUP":       b.projectResourceGroupName(projectName),
		"AZURE_SUBSCRIPTION_ID":      b.driver.SubscriptionID,
		"DEFANG_DEBUG":               os.Getenv("DEFANG_DEBUG"),
		"DEFANG_JSON":                os.Getenv("DEFANG_JSON"),
		"DEFANG_ORG":                 string(b.TenantLabel),
		"DEFANG_PREFIX":              b.Prefix,
		"DEFANG_STATE_URL":           defangStateUrl,
		"HOME":                       "/root",
		"NPM_CONFIG_UPDATE_NOTIFIER": "false",
		"PROJECT":                    projectName,
		pulumiBackendKey:             pulumiBackendValue,          // TODO: make secret
		"PULUMI_CONFIG_PASSPHRASE":   byoc.PulumiConfigPassphrase, // TODO: make secret
		"PULUMI_COPILOT":             "false",
		"PULUMI_SKIP_UPDATE_CHECK":   "true",
		"STACK":                      b.PulumiStack,
		"USER":                       "root",
	}
	if b.kv != nil && b.kv.VaultName != "" {
		env["AZURE_KEY_VAULT_NAME"] = b.kv.VaultName
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
	b.cdLogsDrained = false
	return &client.DeployResponse{
		CdId:   execName,
		CdType: defangv1.CdType_CD_TYPE_AZURE_ACI_JOBID,
		DeployResponse: &defangv1.DeployResponse{
			Etag: etag, Services: serviceInfos,
		},
	}, nil
}

// GetDeploymentStatus implements client.Provider.
//
// When the CD job first reaches a terminal state, we synchronously drain Log Analytics
// for its container output and print it directly to the terminal. This blocks the call
// for up to ~3 minutes while LA finishes ingesting/indexing the logs (which can lag
// significantly behind real time). Doing the drain here — instead of in TailJobLogs —
// keeps the CD-job lifecycle (driven by WaitForCdTaskExit polling this method) and the
// log printing on the same goroutine, so the program doesn't exit before logs flush.
func (b *ByocAzure) GetDeploymentStatus(ctx context.Context) (bool, error) {
	if b.cdRunID == "" {
		return false, nil
	}
	status, err := b.job.GetJobExecutionStatus(ctx, b.cdRunID)
	if err != nil {
		return false, client.ErrDeploymentFailed{Message: err.Error()}
	}
	if !status.IsTerminal() {
		return false, nil
	}

	// First terminal detection: print CD logs from LA before reporting "done".
	if !b.cdLogsDrained {
		b.cdLogsDrained = true
		b.drainCDLogs(ctx)
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

// drainCDLogs polls Log Analytics for container output of b.cdRunID and prints any
// new lines directly to the terminal. Polls for up to drainTotalBudget so freshly-
// terminated executions get a chance for LA ingestion+indexing to catch up. Stops
// early once two consecutive polls yield no new lines (logs have stabilized).
func (b *ByocAzure) drainCDLogs(ctx context.Context) {
	const (
		drainTotalBudget    = 3 * time.Minute
		drainPollInterval   = 5 * time.Second
		consecutiveQuietMax = 2
	)

	deadline := time.Now().Add(drainTotalBudget)
	seen := make(map[string]struct{})
	consecutiveQuiet := 0
	gotAny := false

	for time.Now().Before(deadline) {
		logs, err := b.job.ReadJobLogs(ctx, b.cdRunID)
		if err != nil {
			term.Debugf("drainCDLogs: query error: %v", err)
		} else if logs != "" {
			newLines := 0
			for _, line := range strings.Split(strings.TrimRight(logs, "\n"), "\n") {
				if _, ok := seen[line]; ok {
					continue
				}
				seen[line] = struct{}{}
				term.Println(line)
				newLines++
			}
			if newLines > 0 {
				gotAny = true
				consecutiveQuiet = 0
			} else if gotAny {
				consecutiveQuiet++
			}
		} else if gotAny {
			consecutiveQuiet++
		}

		if gotAny && consecutiveQuiet >= consecutiveQuietMax {
			return
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(drainPollInterval):
		}
	}

	if !gotAny {
		term.Warnf("No CD container output captured from Log Analytics within %s. View later with:"+
			"\n  az monitor log-analytics query --workspace $(az monitor log-analytics workspace show -g %s -n defang-cd --query customerId -o tsv)"+
			" --analytics-query 'ContainerAppConsoleLogs_CL | where ContainerGroupName_s startswith \"%s-\" | order by TimeGenerated asc'",
			drainTotalBudget, b.driver.ResourceGroupName(), b.cdRunID)
	}
}

// GetPrivateDomain implements byoc.ProjectBackend.
func (b *ByocAzure) GetPrivateDomain(projectName string) string {
	return b.GetProjectLabel(projectName) + ".internal"
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
func (b *ByocAzure) ListConfig(ctx context.Context, req *defangv1.ListConfigsRequest) (*defangv1.Secrets, error) {
	if err := b.setUpForConfig(ctx, req.Project); err != nil {
		return nil, err
	}
	prefix := b.StackDir(req.Project, "")
	term.Debugf("Listing App Configuration keys with prefix %q", prefix)
	names, err := b.appCfg.ListSettings(ctx, prefix)
	if err != nil {
		return nil, err
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
	// Write to Key Vault (primary) and App Config (for backward compatibility).
	key := b.StackDir(req.Project, req.Name)
	secretName := keyvault.ToSecretName(key)
	term.Debugf("Putting Key Vault secret %q (original key %q)", secretName, key)
	if err := b.kv.PutSecret(ctx, secretName, req.Value, key); err != nil {
		return fmt.Errorf("failed to put Key Vault secret: %w", err)
	}
	term.Debugf("Putting App Configuration key %q", key)
	return b.appCfg.PutSetting(ctx, key, req.Value)
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
