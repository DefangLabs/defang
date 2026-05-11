package scaleway

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"os"
	"regexp"
	"strings"
	"time"

	"connectrpc.com/connect"
	pkg "github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc/state"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/clouds/scaleway"
	"github.com/DefangLabs/defang/src/pkg/logs"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type ByocScaleway struct {
	*byoc.ByocBaseClient

	client   *scaleway.Client
	s3Client *s3.Client

	region    string
	projectID string

	// CD infrastructure
	bucket           string
	jobDefID         string
	registryEndpoint string

	// CD run tracking
	cdRunID string
	cdEtag  types.ETag

	// Cockpit token for Loki log queries
	cockpitToken        string
	cockpitLogsEndpoint string
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
	// Scaleway secret names must match ^[_a-zA-Z0-9]([-_.a-zA-Z0-9]*[_a-zA-Z0-9])?$
	// Replace path separators from StackDir (e.g., "/Defang/project/stack/KEY") with underscores.
	s := strings.TrimLeft(b.StackDir(projectName, name), "/")
	return strings.ReplaceAll(s, "/", "_")
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
		"AWS_ACCESS_KEY_ID":        b.client.AccessKey, // S3-compatible credentials
		"AWS_SECRET_ACCESS_KEY":    b.client.SecretKey, // S3-compatible credentials
		"AWS_REGION":               b.region,           // needed for S3 client
		"DEFANG_DEBUG":             os.Getenv("DEFANG_DEBUG"),
		"DEFANG_JSON":              os.Getenv("DEFANG_JSON"),
		"DEFANG_ORG":               string(b.TenantLabel),
		"DEFANG_PREFIX":            b.Prefix,
		"DEFANG_PULUMI_DEBUG":      os.Getenv("DEFANG_PULUMI_DEBUG"),
		"DEFANG_PULUMI_DIFF":       os.Getenv("DEFANG_PULUMI_DIFF"),
		"DEFANG_STATE_URL":         defangStateUrl,
		"PRIVATE_DOMAIN":           b.GetPrivateDomain(projectName),
		"PROJECT":                  projectName,
		"PULUMI_CONFIG_PASSPHRASE": byoc.PulumiConfigPassphrase,
		"PULUMI_COPILOT":           "false",
		"PULUMI_HOME":              "/root/.pulumi",
		"PULUMI_SKIP_UPDATE_CHECK": "true",
		"SCW_ACCESS_KEY":           b.client.AccessKey,
		"SCW_SECRET_KEY":           b.client.SecretKey,
		"SCW_DEFAULT_PROJECT_ID":   b.projectID,
		"SCW_DEFAULT_REGION":       b.region,
		"S3_ENDPOINT":              scaleway.S3Endpoint(b.region),
		"STACK":                    b.PulumiStack,
		pulumiBackendKey:           pulumiBackendValue,
	}

	if targets := os.Getenv("DEFANG_PULUMI_TARGETS"); targets != "" {
		env["DEFANG_PULUMI_TARGETS"] = targets
	}
	if !term.StdoutCanColor() {
		env["NO_COLOR"] = "1"
	}
	return env, nil
}

func (b *ByocScaleway) cdSecretName(envName string) string {
	name := fmt.Sprintf("%s-%s-%s", byoc.CdTaskPrefix, b.PulumiStack, envName)
	name = strings.NewReplacer("/", "-", "_", "-").Replace(name)
	if len(name) > 255 {
		name = name[:255]
	}
	return strings.Trim(name, "-")
}

func (b *ByocScaleway) cdJobName() string {
	name := fmt.Sprintf("%s-%s", byoc.CdTaskPrefix, b.PulumiStack)
	name = strings.NewReplacer("/", "-", "_", "-").Replace(name)
	if len(name) > 255 {
		name = name[:255]
	}
	return strings.Trim(name, "-")
}

func cdSecretEnv(env map[string]string) map[string]string {
	keys := []string{
		"AWS_SECRET_ACCESS_KEY",
		"PULUMI_CONFIG_PASSPHRASE",
		"SCW_SECRET_KEY",
	}
	secrets := make(map[string]string, len(keys))
	for _, key := range keys {
		if val, ok := env[key]; ok {
			secrets[key] = val
		}
	}
	return secrets
}

func withoutSecretEnv(env map[string]string) map[string]string {
	secrets := cdSecretEnv(env)
	if len(secrets) == 0 {
		return env
	}
	clean := make(map[string]string, len(env)-len(secrets))
	for key, val := range env {
		if _, ok := secrets[key]; !ok {
			clean[key] = val
		}
	}
	return clean
}

func usesScalewayLLM(project *compose.Project) bool {
	for _, service := range project.Services {
		hasScalewayEndpoint := false
		needsOpenAIKey := false
		for key, val := range service.Environment {
			if val == nil && key == "OPENAI_API_KEY" {
				needsOpenAIKey = true
				continue
			}
			if val != nil && *val == "https://api.scaleway.ai/v1/" {
				hasScalewayEndpoint = true
			}
		}
		if hasScalewayEndpoint && needsOpenAIKey {
			return true
		}
	}
	return false
}

func (b *ByocScaleway) ensureScalewayLLMAuth(ctx context.Context, project *compose.Project) error {
	if !usesScalewayLLM(project) {
		return nil
	}

	configs, err := b.ListConfig(ctx, &defangv1.ListConfigsRequest{Project: project.Name})
	if err != nil {
		return err
	}
	for _, name := range configs.Names {
		if name == "OPENAI_API_KEY" {
			return nil
		}
	}

	term.Infof("Using the Scaleway API key for Managed Inference auth")
	return b.PutConfig(ctx, &defangv1.PutConfigRequest{
		Project: project.Name,
		Name:    "OPENAI_API_KEY",
		Value:   b.client.SecretKey,
	})
}

func (b *ByocScaleway) createCDSecretReferences(ctx context.Context, jobDefID string, env map[string]string) error {
	secretEnv := cdSecretEnv(env)
	if len(secretEnv) == 0 {
		return nil
	}
	refs := make([]scaleway.JobSecretRef, 0, len(secretEnv))
	for _, key := range []string{"AWS_SECRET_ACCESS_KEY", "PULUMI_CONFIG_PASSPHRASE", "SCW_SECRET_KEY"} {
		value, ok := secretEnv[key]
		if !ok {
			continue
		}
		secret, version, err := b.client.EnsureSecretValue(ctx, b.cdSecretName(key), b.projectID, []byte(value))
		if err != nil {
			return scaleway.AnnotateScalewayError(err, fmt.Sprintf("creating CD secret %q", key))
		}
		refs = append(refs, scaleway.JobSecretRef{
			SecretManagerID:      secret.ID,
			SecretManagerVersion: fmt.Sprint(version.Revision),
			EnvVarName:           key,
		})
	}
	return b.client.CreateJobSecrets(ctx, jobDefID, refs)
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

	if os.Getenv("DEFANG_PULUMI_DIR") != "" {
		// Run the cd binary locally from $DEFANG_PULUMI_DIR/cd instead of
		// starting it as a Scaleway Serverless Job. Useful for iterating on cd
		// code without rebuilding/pushing the cd image.
		debugEnv := []string{
			"SCW_ACCESS_KEY=" + b.client.AccessKey,
			"SCW_SECRET_KEY=" + b.client.SecretKey,
			"SCW_DEFAULT_PROJECT_ID=" + b.projectID,
			"SCW_DEFAULT_REGION=" + b.region,
		}
		for k, v := range env {
			debugEnv = append(debugEnv, k+"="+v)
		}
		if err := byoc.DebugPulumiCD(ctx, debugEnv, cmd.command...); err != nil {
			return "", err
		}
		return "local-debug", nil
	}

	run, err := b.client.RunJob(ctx, b.jobDefID, []string{"/app/cd"}, cmd.command, withoutSecretEnv(env))
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

	if err := b.ensureScalewayLLMAuth(ctx, project); err != nil {
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
		if scaleway.IsNotFound(err) || strings.Contains(err.Error(), "NoSuchKey") || strings.Contains(err.Error(), "NoSuchBucket") {
			term.Debug("GetObject:", err)
			return nil, client.ErrNotExist
		}
		return nil, scaleway.AnnotateScalewayError(err, "getting project update")
	}

	var projUpdate defangv1.ProjectUpdate
	if err := proto.Unmarshal(pbBytes, &projUpdate); err != nil {
		term.Debug("Invalid project update:", err)
		return nil, client.ErrNotExist
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
	// getSecretName with empty name gives us the prefix (e.g., "Defang_project_stack_")
	prefix := b.getSecretName(req.Project, "")
	term.Debugf("Listing configs with prefix %q", prefix)

	// Scaleway's name filter does exact matching, not prefix matching.
	// List all secrets in the project and filter client-side.
	secrets, err := b.client.ListSecrets(ctx, b.projectID, "")
	if err != nil {
		return nil, scaleway.AnnotateScalewayError(err, "listing secrets")
	}

	names := make([]string, 0, len(secrets))
	for _, secret := range secrets {
		if strings.HasPrefix(secret.Name, prefix) {
			name := secret.Name[len(prefix):]
			if name != "" {
				names = append(names, name)
			}
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
		found := false
		for _, s := range scwSecrets {
			if s.Name == secretName {
				if err := b.client.DeleteSecret(ctx, s.ID); err != nil {
					return scaleway.AnnotateScalewayError(err, fmt.Sprintf("deleting secret %q", secretName))
				}
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("config not found: %s", name)
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

	if b.CDImage == "" {
		return errors.New("CD image is not set; please set the DEFANG_CD_IMAGE environment variable")
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

	// 3. Create Serverless Job definition for CD tasks (skip in local debug mode)
	if os.Getenv("DEFANG_PULUMI_DIR") == "" {
		jobName := b.cdJobName()
		env, err := b.environment("")
		if err != nil {
			return err
		}
		// Scaleway currently permits duplicate job definitions with the same
		// name. Keep CD setup deterministic by removing every previous Defang
		// CD definition before creating the one this CLI invocation will run.
		defs, err := b.client.ListJobDefinitions(ctx, jobName)
		if err != nil {
			return scaleway.AnnotateScalewayError(err, "listing job definitions")
		}
		for i := range defs {
			if defs[i].Name == jobName {
				if err := b.client.DeleteJobDefinition(ctx, defs[i].ID); err != nil {
					return scaleway.AnnotateScalewayError(err, "deleting stale CD job definition")
				}
			}
		}
		jobDef, err := b.client.CreateJobDefinition(ctx, jobName, b.CDImage, withoutSecretEnv(env), scaleway.JobResources{
			CPULimit:             2000,  // 2 vCPU
			MemoryLimit:          4096,  // 4 GB
			LocalStorageCapacity: 10000, // 10 GB
		})
		if err != nil {
			return scaleway.AnnotateScalewayError(err, "creating CD job definition")
		}
		b.jobDefID = jobDef.ID
		if err := b.createCDSecretReferences(ctx, b.jobDefID, env); err != nil {
			return err
		}
	}

	b.SetupDone = true
	return nil
}

// TearDownCD removes CD infrastructure.
func (b *ByocScaleway) TearDownCD(ctx context.Context) error {
	term.Warn("Deleting the Defang CD infrastructure; existing stacks or configs will not be deleted and will need to be cleaned up manually")

	var errs []error
	if b.jobDefID != "" {
		if err := b.client.DeleteJobDefinition(ctx, b.jobDefID); err != nil {
			errs = append(errs, err)
		}
	}
	defs, err := b.client.ListJobDefinitions(ctx, byoc.CdTaskPrefix)
	if err != nil {
		errs = append(errs, err)
	} else {
		for _, def := range defs {
			if def.Name == byoc.CdTaskPrefix {
				if err := b.client.DeleteJobDefinition(ctx, def.ID); err != nil {
					errs = append(errs, err)
				}
			}
		}
	}

	secretPrefix := b.cdSecretName("")
	secrets, err := b.client.ListSecrets(ctx, b.projectID, secretPrefix)
	if err != nil {
		errs = append(errs, err)
	} else {
		for _, secret := range secrets {
			if strings.HasPrefix(secret.Name, secretPrefix) {
				if err := b.client.DeleteSecret(ctx, secret.ID); err != nil {
					errs = append(errs, err)
				}
			}
		}
	}

	namespaces, err := b.client.ListRegistryNamespaces(ctx, b.projectID, byoc.CdTaskPrefix)
	if err != nil {
		errs = append(errs, err)
	} else {
		for _, ns := range namespaces {
			if ns.Name != byoc.CdTaskPrefix {
				continue
			}
			images, err := b.client.ListImages(ctx, ns.ID)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			for _, image := range images {
				if err := b.client.DeleteImage(ctx, image.ID); err != nil {
					errs = append(errs, err)
				}
			}
			if err := b.client.DeleteRegistryNamespace(ctx, ns.ID); err != nil {
				errs = append(errs, err)
			}
		}
	}

	if err := scaleway.EmptyAndDeleteBucket(ctx, b.s3Client, b.bucketName()); err != nil {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		for _, err := range errs {
			term.Warnf("Failed to delete Scaleway CD resource: %v", err)
		}
		return errors.Join(errs...)
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
	case "failed", "interrupted":
		msg := fmt.Sprintf("CD job %s: %s", run.State, run.ErrorMessage)
		if run.ErrorMessage == "" {
			msg = fmt.Sprintf("CD job %s: %s", run.State, run.Reason)
		}
		return true, client.ErrDeploymentFailed{Message: msg}
	default:
		// still running: queued, running, etc.
		return false, nil
	}
}

func (b *ByocScaleway) PrepareDomainDelegation(ctx context.Context, req client.PrepareDomainDelegationRequest) (*client.PrepareDomainDelegationResponse, error) {
	term.Debugf("Preparing domain delegation for %s", req.DelegateDomain)

	domain, subdomain := splitDelegateDomain(req.DelegateDomain)
	zone, err := b.client.CreateDNSZone(ctx, domain, subdomain, b.projectID)
	if err != nil {
		if !scaleway.IsConflict(err) {
			// Domain not owned by this Scaleway project — skip delegation
			// (services will use Scaleway's auto-generated container URLs)
			term.Debugf("Skipping domain delegation: %v", err)
			return nil, nil
		}
		// Zone already exists; look it up
		zone, err = b.client.GetDNSZone(ctx, req.DelegateDomain)
		if err != nil {
			return nil, err
		}
	}

	if len(zone.NS) == 0 {
		return nil, fmt.Errorf("DNS zone for %q has no nameservers", req.DelegateDomain)
	}

	term.Debugf("DNS zone for %q has nameservers: %v", req.DelegateDomain, zone.NS)
	return &client.PrepareDomainDelegationResponse{
		NameServers: zone.NS,
	}, nil
}

// splitDelegateDomain splits a FQDN into its base domain and subdomain parts.
// For example, "myapp.example.com" returns ("example.com", "myapp").
func splitDelegateDomain(fqdn string) (domain, subdomain string) {
	fqdn = strings.TrimSuffix(fqdn, ".")
	parts := strings.Split(fqdn, ".")
	if len(parts) <= 2 {
		return fqdn, ""
	}
	domain = strings.Join(parts[len(parts)-2:], ".")
	subdomain = strings.Join(parts[:len(parts)-2], ".")
	return
}

func (b *ByocScaleway) Subscribe(ctx context.Context, req *defangv1.SubscribeRequest) (iter.Seq2[*defangv1.SubscribeResponse, error], error) {
	if b.cdRunID == "" || (req.Etag != "" && req.Etag != b.cdEtag) {
		return nil, errors.ErrUnsupported
	}

	runID := b.cdRunID
	return func(yield func(*defangv1.SubscribeResponse, error) bool) {
		var lastState string
		for {
			run, err := b.client.GetJobRun(ctx, runID)
			if err != nil {
				yield(nil, scaleway.AnnotateScalewayError(err, "polling job run status"))
				return
			}

			if run.State != lastState {
				lastState = run.State
				state := jobRunStateToServiceState(run.State)
				if !yield(&defangv1.SubscribeResponse{
					Name:   "cd",
					Status: run.State,
					State:  state,
				}, nil) {
					return
				}

				// Stop on terminal states
				if state == defangv1.ServiceState_DEPLOYMENT_COMPLETED ||
					state == defangv1.ServiceState_DEPLOYMENT_FAILED {
					return
				}
			}

			if err := pkg.SleepWithContext(ctx, 2*time.Second); err != nil {
				yield(nil, err)
				return
			}
		}
	}, nil
}

func jobRunStateToServiceState(state string) defangv1.ServiceState {
	switch state {
	case "initialized", "validated", "queued", "retrying":
		return defangv1.ServiceState_UPDATE_QUEUED
	case "running":
		return defangv1.ServiceState_DEPLOYMENT_PENDING
	case "succeeded":
		return defangv1.ServiceState_DEPLOYMENT_COMPLETED
	case "failed", "interrupted":
		return defangv1.ServiceState_DEPLOYMENT_FAILED
	default:
		return defangv1.ServiceState_NOT_SPECIFIED
	}
}

func (b *ByocScaleway) QueryLogs(ctx context.Context, req *defangv1.TailRequest) (iter.Seq2[*defangv1.TailResponse, error], error) {
	if err := b.ensureCockpitToken(ctx); err != nil {
		return nil, err
	}

	query := b.buildLogQuery(req)
	etag := req.Etag
	if etag == "" {
		etag = b.cdEtag
	}

	if req.Follow {
		return b.followLogs(ctx, query, etag, req), nil
	}

	// Non-follow: single query
	var start, end time.Time
	if req.Since.IsValid() {
		start = req.Since.AsTime()
	}
	if req.Until.IsValid() {
		end = req.Until.AsTime()
	}

	limit := int(req.Limit)
	if limit == 0 {
		limit = 100
	}

	entries, err := scaleway.QueryLoki(ctx, b.cockpitToken, b.cockpitLogsEndpoint, query, start, end, limit)
	if err != nil {
		return nil, fmt.Errorf("querying logs: %w", err)
	}

	return func(yield func(*defangv1.TailResponse, error) bool) {
		for _, entry := range entries {
			resp := lokiEntryToTailResponse(entry, etag)
			if resp == nil {
				continue
			}
			if !yield(resp, nil) {
				return
			}
		}
	}, nil
}

// ensureCockpitToken lazily creates a Cockpit token for Loki queries.
func (b *ByocScaleway) ensureCockpitToken(ctx context.Context) error {
	if b.cockpitToken != "" && b.cockpitLogsEndpoint != "" {
		return nil
	}

	const tokenName = "defang-cd-logs"

	token, err := b.client.CreateCockpitToken(ctx, tokenName, b.projectID)
	if err != nil {
		if !scaleway.IsConflict(err) {
			return fmt.Errorf("creating Cockpit token: %w", err)
		}
		// Token exists but we need the secret key; delete and recreate
		tokens, listErr := b.client.ListCockpitTokens(ctx, b.projectID)
		if listErr != nil {
			return fmt.Errorf("listing Cockpit tokens: %w", listErr)
		}
		for _, t := range tokens {
			if t.Name == tokenName {
				if delErr := b.client.DeleteCockpitToken(ctx, t.ID); delErr != nil {
					return fmt.Errorf("deleting existing Cockpit token: %w", delErr)
				}
				break
			}
		}
		// Recreate to obtain the secret key
		token, err = b.client.CreateCockpitToken(ctx, tokenName, b.projectID)
		if err != nil {
			return fmt.Errorf("recreating Cockpit token: %w", err)
		}
	}

	if b.cockpitToken == "" {
		b.cockpitToken = token.SecretKey
	}
	if b.cockpitLogsEndpoint == "" {
		endpoint, err := b.client.GetCockpitLogsEndpoint(ctx, b.projectID)
		if err != nil {
			return err
		}
		b.cockpitLogsEndpoint = endpoint
	}
	return nil
}

// buildLogQuery constructs a LogQL query for Scaleway Cockpit Loki.
func (b *ByocScaleway) buildLogQuery(req *defangv1.TailRequest) string {
	logType := logs.LogType(req.LogType)

	if len(req.Services) > 0 {
		if len(req.Services) == 1 {
			return fmt.Sprintf(`{resource_type="serverless_container",resource_name=~".*-%s"}`, regexp.QuoteMeta(req.Services[0]))
		}
		services := make([]string, len(req.Services))
		for i, service := range req.Services {
			services[i] = regexp.QuoteMeta(service)
		}
		return fmt.Sprintf(`{resource_type="serverless_container",resource_name=~".*-(%s)"}`, strings.Join(services, "|"))
	}

	if logType.Has(logs.LogTypeCD) || logType == logs.LogTypeUnspecified {
		return fmt.Sprintf(`{job_definition_name=%q}`, b.cdJobName())
	}

	return fmt.Sprintf(`{job_definition_name=~"%s.*"}`, byoc.CdTaskPrefix)
}

// followLogs polls Loki for new log entries in a loop.
func (b *ByocScaleway) followLogs(ctx context.Context, query, etag string, req *defangv1.TailRequest) iter.Seq2[*defangv1.TailResponse, error] {
	return func(yield func(*defangv1.TailResponse, error) bool) {
		start := time.Now()
		if req.Since.IsValid() {
			start = req.Since.AsTime()
		}

		const maxConsecutiveErrors = 5
		consecutiveErrors := 0

		for {
			entries, err := scaleway.QueryLoki(ctx, b.cockpitToken, b.cockpitLogsEndpoint, query, start, time.Time{}, 100)
			if err != nil {
				consecutiveErrors++
				if consecutiveErrors >= maxConsecutiveErrors {
					yield(nil, fmt.Errorf("giving up after %d consecutive log query failures: %w", maxConsecutiveErrors, err))
					return
				}
				if !yield(nil, err) {
					return
				}
			} else {
				consecutiveErrors = 0
			}

			for _, entry := range entries {
				if !entry.Timestamp.After(start) {
					continue // skip already-seen entries
				}
				resp := lokiEntryToTailResponse(entry, etag)
				if resp == nil {
					start = entry.Timestamp
					continue
				}
				if !yield(resp, nil) {
					return
				}
				start = entry.Timestamp
			}

			if err := pkg.SleepWithContext(ctx, 2*time.Second); err != nil {
				yield(nil, err)
				return
			}
		}
	}
}

type scalewayLogPayload struct {
	JobDefinitionName string `json:"job_definition_name"`
	Message           string `json:"message"`
	ResourceID        string `json:"resource_id"`
	ResourceInstance  string `json:"resource_instance"`
	ResourceName      string `json:"resource_name"`
	Stream            string `json:"stream"`
}

func parseScalewayLogEntry(entry scaleway.LokiEntry) (scaleway.LokiEntry, string) {
	var payload scalewayLogPayload
	if err := json.Unmarshal([]byte(entry.Line), &payload); err != nil {
		return entry, entry.Line
	}
	if entry.Labels == nil {
		entry.Labels = map[string]string{}
	}
	if payload.Message != "" {
		entry.Line = payload.Message
	}
	if payload.ResourceName != "" {
		entry.Labels["resource_name"] = payload.ResourceName
	}
	if payload.JobDefinitionName != "" {
		entry.Labels["job_definition_name"] = payload.JobDefinitionName
	}
	if payload.ResourceID != "" {
		entry.Labels["resource_id"] = payload.ResourceID
	}
	if payload.ResourceInstance != "" {
		entry.Labels["resource_instance"] = payload.ResourceInstance
	}
	if payload.Stream != "" {
		entry.Labels["stream"] = payload.Stream
	}
	return entry, payload.Message
}

func lokiEntryToTailResponse(entry scaleway.LokiEntry, etag string) *defangv1.TailResponse {
	entry, message := parseScalewayLogEntry(entry)
	if message == "" {
		return nil
	}
	service := entry.Labels["resource_name"]
	if service == "" {
		service = entry.Labels["job_definition_name"]
	}
	if service == "" {
		service = "cd"
	}
	host := entry.Labels["resource_instance"]
	if host == "" {
		host = entry.Labels["resource_id"]
	}
	stderr := entry.Labels["stream"] == "stderr" || strings.Contains(strings.ToLower(message), "error")

	return &defangv1.TailResponse{
		Service: service,
		Etag:    etag,
		Entries: []*defangv1.LogEntry{{
			Message:   message,
			Timestamp: timestamppb.New(entry.Timestamp),
			Service:   service,
			Etag:      etag,
			Host:      host,
			Stderr:    stderr,
		}},
	}
}
