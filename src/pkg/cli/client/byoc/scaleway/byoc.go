package scaleway

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"iter"
	"os"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc/state"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/clouds/scaleway"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"google.golang.org/protobuf/proto"
)

type ByocScaleway struct {
	*byoc.ByocBaseClient

	client   *scaleway.Client
	s3Client *s3.Client

	region    string
	projectID string

	// CD infrastructure
	bucket       string
	jobDefID     string
	registryEndpoint string

	// CD run tracking
	cdRunID string
	cdEtag  types.ETag
}

var _ client.Provider = (*ByocScaleway)(nil)

func NewByocProvider(ctx context.Context, tenantLabel types.TenantLabel, stack string) *ByocScaleway {
	b := &ByocScaleway{
		region: os.Getenv("SCW_DEFAULT_REGION"),
	}
	b.ByocBaseClient = byoc.NewByocBaseClient(tenantLabel, b, stack)
	return b
}

func (*ByocScaleway) Driver() string {
	return "scaleway-jobs"
}

func (b *ByocScaleway) Authenticate(ctx context.Context, interactive bool) error {
	scwClient, err := scaleway.NewClientFromEnv()
	if err != nil {
		return err
	}

	if _, err := scwClient.Authenticate(ctx); err != nil {
		return err
	}

	b.client = scwClient
	b.projectID = scwClient.ProjectID
	b.region = scwClient.Region

	b.s3Client = scaleway.NewS3Client(scwClient.Region, scwClient.AccessKey, scwClient.SecretKey)
	return nil
}

func (b *ByocScaleway) AccountInfo(ctx context.Context) (*client.AccountInfo, error) {
	if b.client == nil {
		return nil, errors.New("not authenticated; call Authenticate first")
	}
	info := b.client.GetAccountInfo()
	return &client.AccountInfo{
		AccountID: info.ProjectID,
		Provider:  client.ProviderScaleway,
		Region:    info.Region,
	}, nil
}

func (b *ByocScaleway) bucketName() string {
	if b.bucket != "" {
		return b.bucket
	}
	return fmt.Sprintf("%s-%s", byoc.CdTaskPrefix, b.PulumiStack)
}

func (b *ByocScaleway) getSecretName(projectName, name string) string {
	return b.StackDir(projectName, name)
}

func (b *ByocScaleway) environment(projectName string) (map[string]string, error) {
	// From https://www.pulumi.com/docs/iac/concepts/state-and-backends/#aws-s3
	// Scaleway S3-compatible storage uses the same s3:// scheme with custom endpoint
	defangStateUrl := fmt.Sprintf("s3://%s?endpoint=%s&disableSSL=false&s3ForcePathStyle=true", b.bucketName(), scaleway.S3Endpoint(b.region))
	pulumiBackendKey, pulumiBackendValue, err := byoc.GetPulumiBackend(defangStateUrl)
	if err != nil {
		return nil, err
	}
	env := map[string]string{
		"AWS_ACCESS_KEY_ID":          b.client.AccessKey,  // S3-compatible credentials
		"AWS_SECRET_ACCESS_KEY":      b.client.SecretKey,  // S3-compatible credentials
		"AWS_REGION":                 b.region,            // needed for S3 client
		"DEFANG_DEBUG":               os.Getenv("DEFANG_DEBUG"),
		"DEFANG_JSON":                os.Getenv("DEFANG_JSON"),
		"DEFANG_ORG":                 string(b.TenantLabel),
		"DEFANG_PREFIX":              b.Prefix,
		"DEFANG_PULUMI_DEBUG":        os.Getenv("DEFANG_PULUMI_DEBUG"),
		"DEFANG_PULUMI_DIFF":         os.Getenv("DEFANG_PULUMI_DIFF"),
		"DEFANG_STATE_URL":           defangStateUrl,
		"NODE_NO_WARNINGS":           "1",
		"NPM_CONFIG_UPDATE_NOTIFIER": "false",
		"PRIVATE_DOMAIN":             b.GetPrivateDomain(projectName),
		"PROJECT":                    projectName,
		"PULUMI_CONFIG_PASSPHRASE":   byoc.PulumiConfigPassphrase,
		"PULUMI_COPILOT":             "false",
		"PULUMI_SKIP_UPDATE_CHECK":   "true",
		"SCW_ACCESS_KEY":             b.client.AccessKey,
		"SCW_SECRET_KEY":             b.client.SecretKey,
		"SCW_DEFAULT_PROJECT_ID":     b.projectID,
		"SCW_DEFAULT_REGION":         b.region,
		"STACK":                      b.PulumiStack,
		pulumiBackendKey:             pulumiBackendValue,
	}

	if targets := os.Getenv("DEFANG_PULUMI_TARGETS"); targets != "" {
		env["DEFANG_PULUMI_TARGETS"] = targets
	}
	if !term.StdoutCanColor() {
		env["NO_COLOR"] = "1"
	}
	return env, nil
}

type cdCommand struct {
	command        []string
	delegateDomain string
	etag           types.ETag
	mode           defangv1.DeploymentMode
	project        string
	statesUrl      string
	eventsUrl      string
}

func (b *ByocScaleway) runCdCommand(ctx context.Context, cmd cdCommand) (string, error) {
	env, err := b.environment(cmd.project)
	if err != nil {
		return "", err
	}

	if cmd.delegateDomain != "" {
		env["DOMAIN"] = cmd.delegateDomain
	} else {
		env["DOMAIN"] = "dummy.domain"
	}
	if cmd.etag != "" {
		env["DEFANG_ETAG"] = cmd.etag
	}
	env["DEFANG_MODE"] = strings.ToLower(cmd.mode.String())
	if cmd.statesUrl != "" {
		env["DEFANG_STATES_UPLOAD_URL"] = cmd.statesUrl
	}
	if cmd.eventsUrl != "" {
		env["DEFANG_EVENTS_UPLOAD_URL"] = cmd.eventsUrl
	}

	// Build the command as entrypoint + args
	args := append([]string{"node", "lib/index.js"}, cmd.command...)
	env["DEFANG_CD_CMD"] = strings.Join(args, " ")

	run, err := b.client.RunJob(ctx, b.jobDefID, env)
	if err != nil {
		return "", scaleway.AnnotateScalewayError(err, "running CD command")
	}
	return run.ID, nil
}

// Deploy implements the Provider interface.
func (b *ByocScaleway) Deploy(ctx context.Context, req *client.DeployRequest) (*client.DeployResponse, error) {
	return b.deploy(ctx, req, "up")
}

// Preview implements the Provider interface.
func (b *ByocScaleway) Preview(ctx context.Context, req *client.DeployRequest) (*client.DeployResponse, error) {
	return b.deploy(ctx, req, "preview")
}

func (b *ByocScaleway) deploy(ctx context.Context, req *client.DeployRequest, cmd string) (*client.DeployResponse, error) {
	project, err := compose.LoadFromContent(ctx, req.Compose, "")
	if err != nil {
		return nil, err
	}

	if err := b.SetUpCD(ctx, false); err != nil {
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
	})
	if err != nil {
		return nil, err
	}

	var payloadString string
	if len(data) < 1000 {
		payloadString = base64.StdEncoding.EncodeToString(data)
	} else {
		bucket := b.bucketName()
		key := fmt.Sprintf("uploads/%s", etag)
		if err := scaleway.PutObject(ctx, b.s3Client, bucket, key, bytes.NewReader(data)); err != nil {
			return nil, fmt.Errorf("uploading deploy payload: %w", err)
		}
		payloadString = fmt.Sprintf("s3://%s/%s", bucket, key)
	}

	cdCmd := cdCommand{
		command:        []string{cmd, payloadString},
		delegateDomain: req.DelegateDomain,
		etag:           etag,
		mode:           req.Mode,
		project:        project.Name,
		statesUrl:      req.StatesUrl,
		eventsUrl:      req.EventsUrl,
	}
	runID, err := b.runCdCommand(ctx, cdCmd)
	if err != nil {
		return nil, err
	}
	b.cdEtag = etag
	b.cdRunID = runID

	return &client.DeployResponse{
		CdType: defangv1.CdType_CD_TYPE_UNSPECIFIED, // No Scaleway-specific CdType yet
		CdId:   runID,
		DeployResponse: &defangv1.DeployResponse{
			Services: serviceInfos,
			Etag:     etag,
		},
	}, nil
}

// GetProjectUpdate downloads the project state from S3.
func (b *ByocScaleway) GetProjectUpdate(ctx context.Context, projectName string) (*defangv1.ProjectUpdate, error) {
	if projectName == "" {
		return nil, client.ErrNotExist
	}

	bucket := b.bucketName()
	path := b.GetProjectUpdatePath(projectName)

	term.Debug("Getting services from bucket:", bucket, path)
	pbBytes, err := scaleway.GetObject(ctx, b.s3Client, bucket, path)
	if err != nil {
		if scaleway.IsNotFound(err) || strings.Contains(err.Error(), "NoSuchKey") {
			term.Debug("GetObject:", err)
			return nil, client.ErrNotExist
		}
		return nil, scaleway.AnnotateScalewayError(err, "getting project update")
	}

	var projUpdate defangv1.ProjectUpdate
	if err := proto.Unmarshal(pbBytes, &projUpdate); err != nil {
		return nil, err
	}
	return &projUpdate, nil
}

// GetServices implements the Provider interface.
func (b *ByocScaleway) GetServices(ctx context.Context, req *defangv1.GetServicesRequest) (*defangv1.GetServicesResponse, error) {
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

// GetService implements the Provider interface.
func (b *ByocScaleway) GetService(ctx context.Context, req *defangv1.GetRequest) (*defangv1.ServiceInfo, error) {
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

// PutConfig stores a secret in Scaleway Secret Manager.
func (b *ByocScaleway) PutConfig(ctx context.Context, req *defangv1.PutConfigRequest) error {
	secretName := b.getSecretName(req.Project, req.Name)
	term.Debugf("Putting config %q", secretName)

	// Try to create the secret first; if it already exists, we'll just add a version
	secret, err := b.client.CreateSecret(ctx, secretName, b.projectID)
	if err != nil {
		if !scaleway.IsConflict(err) {
			return scaleway.AnnotateScalewayError(err, fmt.Sprintf("creating secret %q", secretName))
		}
		// Secret already exists; find it to get the ID
		secrets, listErr := b.client.ListSecrets(ctx, b.projectID, secretName)
		if listErr != nil {
			return scaleway.AnnotateScalewayError(listErr, fmt.Sprintf("listing secrets for %q", secretName))
		}
		for i := range secrets {
			if secrets[i].Name == secretName {
				secret = &secrets[i]
				break
			}
		}
		if secret == nil {
			return fmt.Errorf("secret %q exists but could not be found", secretName)
		}
	}

	if _, err := b.client.CreateSecretVersion(ctx, secret.ID, []byte(req.Value)); err != nil {
		return scaleway.AnnotateScalewayError(err, fmt.Sprintf("adding version for secret %q", secretName))
	}
	return nil
}

// ListConfig lists secrets from Scaleway Secret Manager.
func (b *ByocScaleway) ListConfig(ctx context.Context, req *defangv1.ListConfigsRequest) (*defangv1.Secrets, error) {
	prefix := b.getSecretName(req.Project, "")
	term.Debugf("Listing configs with prefix %q", prefix)

	secrets, err := b.client.ListSecrets(ctx, b.projectID, prefix)
	if err != nil {
		return nil, scaleway.AnnotateScalewayError(err, "listing secrets")
	}

	names := make([]string, 0, len(secrets))
	for _, secret := range secrets {
		name := strings.TrimPrefix(secret.Name, prefix)
		if name != "" {
			names = append(names, name)
		}
	}
	return &defangv1.Secrets{Names: names}, nil
}

// DeleteConfig deletes secrets from Scaleway Secret Manager.
func (b *ByocScaleway) DeleteConfig(ctx context.Context, secrets *defangv1.Secrets) error {
	for _, name := range secrets.Names {
		secretName := b.getSecretName(secrets.Project, name)
		term.Debugf("Deleting config %q", secretName)

		// Need to find the secret ID by name
		scwSecrets, err := b.client.ListSecrets(ctx, b.projectID, secretName)
		if err != nil {
			return scaleway.AnnotateScalewayError(err, fmt.Sprintf("listing secrets for %q", secretName))
		}
		for _, s := range scwSecrets {
			if s.Name == secretName {
				if err := b.client.DeleteSecret(ctx, s.ID); err != nil {
					return scaleway.AnnotateScalewayError(err, fmt.Sprintf("deleting secret %q", secretName))
				}
				break
			}
		}
	}
	return nil
}

// CreateUploadURL generates a presigned URL for uploading artifacts.
func (b *ByocScaleway) CreateUploadURL(ctx context.Context, req *defangv1.UploadURLRequest) (*defangv1.UploadURLResponse, error) {
	if err := b.SetUpCD(ctx, false); err != nil {
		return nil, err
	}

	bucket := b.bucketName()
	key := fmt.Sprintf("uploads/%s", req.Digest)
	url, err := scaleway.CreatePresignedUploadURL(ctx, b.s3Client, bucket, key, 15*time.Minute)
	if err != nil {
		return nil, scaleway.AnnotateScalewayError(err, "creating upload URL")
	}
	return &defangv1.UploadURLResponse{Url: url}, nil
}

// SetUpCD creates the infrastructure needed for CD operations.
func (b *ByocScaleway) SetUpCD(ctx context.Context, force bool) error {
	if b.SetupDone && !force {
		return nil
	}

	term.Debugf("Using CD image: %q", b.CDImage)

	bucket := b.bucketName()
	term.Infof("Setting up Defang CD in Scaleway project %s", b.projectID)

	// 1. Create S3 bucket for state/artifacts
	if err := scaleway.EnsureBucketExists(ctx, b.s3Client, bucket, b.region); err != nil {
		return scaleway.AnnotateScalewayError(err, "ensuring CD bucket exists")
	}
	b.bucket = bucket

	// 2. Create Container Registry namespace
	registryName := byoc.CdTaskPrefix
	ns, err := b.client.EnsureRegistryNamespaceExists(ctx, registryName, b.projectID, b.region)
	if err != nil {
		return scaleway.AnnotateScalewayError(err, "ensuring registry namespace exists")
	}
	b.registryEndpoint = ns.Endpoint

	// 3. Create Serverless Job definition for CD tasks
	jobName := byoc.CdTaskPrefix
	env, err := b.environment("")
	if err != nil {
		return err
	}
	jobDef, err := b.client.CreateJobDefinition(ctx, jobName, b.CDImage, env, scaleway.JobResources{
		CPULimit:    2000,  // 2 vCPU
		MemoryLimit: 4096,  // 4 GB
	})
	if err != nil {
		if scaleway.IsConflict(err) {
			// Job definition already exists; find it
			// List and find by name is not directly supported, so we store the ID
			term.Debug("CD job definition already exists")
		} else {
			return scaleway.AnnotateScalewayError(err, "creating CD job definition")
		}
	} else {
		b.jobDefID = jobDef.ID
	}

	b.SetupDone = true
	return nil
}

// TearDownCD removes CD infrastructure.
func (b *ByocScaleway) TearDownCD(ctx context.Context) error {
	// TODO: implement full teardown (delete job definition, registry namespace, bucket)
	term.Warn("Deleting the Defang CD infrastructure; existing stacks or configs will not be deleted and will need to be cleaned up manually")

	if b.jobDefID != "" {
		if err := b.client.DeleteJobDefinition(ctx, b.jobDefID); err != nil {
			term.Warnf("Failed to delete CD job definition: %v", err)
		}
	}
	return nil
}

// CdCommand runs a CD command via Serverless Jobs.
func (b *ByocScaleway) CdCommand(ctx context.Context, req client.CdCommandRequest) (*client.CdCommandResponse, error) {
	if err := b.SetUpCD(ctx, false); err != nil {
		return nil, err
	}
	etag := types.NewEtag()
	cmd := cdCommand{
		command:   []string{string(req.Command)},
		etag:      etag,
		project:   req.Project,
		statesUrl: req.StatesUrl,
		eventsUrl: req.EventsUrl,
	}
	runID, err := b.runCdCommand(ctx, cmd)
	if err != nil {
		return nil, err
	}
	b.cdEtag = etag
	b.cdRunID = runID
	return &client.CdCommandResponse{
		ETag:   etag,
		CdType: defangv1.CdType_CD_TYPE_UNSPECIFIED,
		CdId:   runID,
	}, nil
}

// CdList lists Pulumi stacks from the S3 state bucket.
func (b *ByocScaleway) CdList(ctx context.Context, _ bool) (iter.Seq[state.Info], error) {
	bucket := b.bucketName()
	prefix := ".pulumi/stacks/"

	term.Debug("Listing stacks in bucket:", bucket, prefix)
	keys, err := scaleway.ListObjectKeys(ctx, b.s3Client, bucket, prefix)
	if err != nil {
		return nil, scaleway.AnnotateScalewayError(err, "listing Pulumi stacks")
	}

	objLoader := func(ctx context.Context, path string) ([]byte, error) {
		return scaleway.GetObject(ctx, b.s3Client, bucket, path)
	}

	return func(yield func(state.Info) bool) {
		for _, key := range keys {
			data, err := objLoader(ctx, key)
			if err != nil {
				term.Debugf("Skipping %q in bucket %s: %v", key, bucket, err)
				continue
			}
			st, err := state.ParsePulumiStateFile(ctx, s3Obj{name: key, size: int64(len(data))}, func(ctx context.Context, _ string) ([]byte, error) {
				return data, nil
			})
			if err != nil {
				term.Debugf("Skipping %q in bucket %s: %v", key, bucket, err)
				continue
			}
			if st == nil {
				continue
			}
			info := state.Info{
				Stack:     st.Name,
				Project:   st.Project,
				Workspace: string(st.Workspace),
				CdRegion:  b.region,
			}
			if !yield(info) {
				break
			}
		}
	}, nil
}

// s3Obj implements state.BucketObj for CdList.
type s3Obj struct {
	name string
	size int64
}

func (o s3Obj) Name() string { return o.name }
func (o s3Obj) Size() int64  { return o.size }

func (b *ByocScaleway) GetPrivateDomain(projectName string) string {
	return fmt.Sprintf("%s.internal", projectName)
}

// GetDeploymentStatus checks the status of the CD job run.
func (b *ByocScaleway) GetDeploymentStatus(ctx context.Context) (bool, error) {
	if b.cdRunID == "" {
		return false, errors.New("no CD run in progress")
	}

	run, err := b.client.GetJobRun(ctx, b.cdRunID)
	if err != nil {
		return false, scaleway.AnnotateScalewayError(err, "getting deployment status")
	}

	switch run.State {
	case "succeeded":
		return true, nil
	case "failed", "canceled":
		msg := fmt.Sprintf("CD job %s: %s", run.State, run.ErrorMessage)
		return true, client.ErrDeploymentFailed{Message: msg}
	default:
		// still running: queued, running, etc.
		return false, nil
	}
}

func (b *ByocScaleway) PrepareDomainDelegation(ctx context.Context, req client.PrepareDomainDelegationRequest) (*client.PrepareDomainDelegationResponse, error) {
	return nil, client.ErrNotImplemented("Scaleway PrepareDomainDelegation")
}

func (b *ByocScaleway) QueryLogs(ctx context.Context, req *defangv1.TailRequest) (iter.Seq2[*defangv1.TailResponse, error], error) {
	return nil, client.ErrNotImplemented("Scaleway QueryLogs")
}

func (b *ByocScaleway) Subscribe(ctx context.Context, req *defangv1.SubscribeRequest) (iter.Seq2[*defangv1.SubscribeResponse, error], error) {
	return nil, client.ErrNotImplemented("Scaleway Subscribe")
}
