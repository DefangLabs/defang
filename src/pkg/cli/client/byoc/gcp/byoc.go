package gcp

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws/ecs"
	"github.com/DefangLabs/defang/src/pkg/clouds/gcp"
	"github.com/DefangLabs/defang/src/pkg/http"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/aws/smithy-go/ptr"
	composeTypes "github.com/compose-spec/compose-go/v2/types"
	"google.golang.org/protobuf/proto"
)

var _ client.Provider = (*ByocGcp)(nil)

const (
	DefangCDProjectName = "defang-cd"
)

var (
	DefaultCDTags    = map[string]string{"created-by": "defang"}
	PulumiVersion    = pkg.Getenv("DEFANG_PULUMI_VERSION", "3.136.1")
	DefangGcpCdImage = pkg.Getenv("DEFANG_GCP_CD_IMAGE", "edwardrf/gcpcd:test")

	//TODO: Adjust permissions
	cdPermissions = []string{
		"run.operations.get",
		"run.operations.list",
		"run.routes.get",
		"run.routes.invoke",
		"run.routes.list",
		"run.services.create",
		"run.services.delete",
		"run.services.get",
		"run.services.getIamPolicy",
		"run.services.list",
		"run.services.listEffectiveTags",
		"run.services.listTagBindings",
		"run.services.update",
	}
)

type ByocGcp struct {
	*byoc.ByocBaseClient

	driver *gcp.Gcp

	bucket               string
	cdServiceAccount     string
	registry             string
	role                 string
	setupDone            bool
	uploadServiceAccount string

	lastCdExecution string
	lastCdEtag      string
}

func NewByocProvider(ctx context.Context, tenantId types.TenantID) *ByocGcp {
	region := pkg.Getenv("GCP_REGION", "us-central1") // Defaults to us-central1 for lower price
	projectId := os.Getenv("GCP_PROJECT_ID")
	b := &ByocGcp{driver: &gcp.Gcp{Region: region, ProjectId: projectId}}
	b.ByocBaseClient = byoc.NewByocBaseClient(ctx, tenantId, b)
	return b
}

func (b *ByocGcp) setUpCD(ctx context.Context) error {
	if b.setupDone {
		return nil
	}
	// TODO: Handle organizations and Project Creation

	// 1. Enable required APIs
	apis := []string{
		"storage.googleapis.com",              // Cloud Storage API
		"artifactregistry.googleapis.com",     // Artifact Registry API
		"run.googleapis.com",                  // Cloud Run API
		"iam.googleapis.com",                  // IAM API
		"cloudresourcemanager.googleapis.com", // For service account and role management
	}
	if err := b.driver.EnsureAPIsEnabled(ctx, apis...); err != nil {
		if strings.Contains(err.Error(), "Service Usage API has not been used in project") {
			term.Warn("Service Usage API has not been used in project, we cannot verify if the needed APIs are enabled. Please enable the APIs manually.")
			start := strings.Index(err.Error(), "https://console.developers.google.com")
			end := strings.Index(err.Error(), "Details:") - 1
			if start >= 0 && end >= 0 {
				term.Warn("To enable service usage API, " + err.Error()[start:end])
			}
		} else {
			return err
		}
	}

	// 2. Setup cd bucket
	if bucket, err := b.driver.EnsureBucketExists(ctx, "defang-cd"); err != nil {
		return err
	} else {
		b.bucket = bucket
	}

	// 3. Setup Artifact Registry
	if registry, err := b.driver.EnsureArtifactRegistryExists(ctx, "defang-cd"); err != nil {
		return err
	} else {
		b.registry = registry
	}

	// 4. Setup Service Accounts and its permissions to be used by the CD job and pre-signed URL generation
	//   4.1 CD role should be able to work with cloudrun services
	if role, err := b.driver.EnsureRoleExists(ctx, "defang_cd_role", "defang CD", "defang CD deployment role", cdPermissions); err != nil {
		return err
	} else {
		b.role = role
	}
	if cdServiceAccount, err := b.driver.EnsureServiceAccountExists(ctx, "defang-cd", "defang CD", "Service account for defang CD"); err != nil {
		return err
	} else {
		b.cdServiceAccount = path.Base(cdServiceAccount)
	}
	if err := b.driver.EnsureServiceAccountHasRoles(ctx, b.cdServiceAccount, []string{b.role, "roles/iam.serviceAccountAdmin"}); err != nil {
		return err
	}
	//  4.2 Give cd role access to cd bucket
	if err := b.driver.EnsureServiceAccountHasBucketRoles(ctx, b.bucket, b.cdServiceAccount, []string{"roles/storage.objectAdmin"}); err != nil {
		return err
	}
	//  4.3 Give cd role access to artifact registry
	if err := b.driver.EnsureServiceAccountHasArtifactRegistryRoles(ctx, b.registry, b.cdServiceAccount, []string{"roles/artifactregistry.repoAdmin"}); err != nil {
		return err
	}
	//  4.4 Setup service account for upload
	if uploadServiceAccount, err := b.driver.EnsureServiceAccountExists(ctx, "defang-upload", "defang upload", "Service account for defang cli to generate pre-signed URL to upload artifacts"); err != nil {
		return err
	} else {
		b.uploadServiceAccount = path.Base(uploadServiceAccount)
	}
	//  4.5 Give upload service account access to cd bucket
	if err := b.driver.EnsureServiceAccountHasBucketRoles(ctx, b.bucket, b.uploadServiceAccount, []string{"roles/storage.objectUser"}); err != nil {
		return err
	}
	//  4.6 Give current user the token creator role on the upload service account
	user, err := b.driver.GetCurrentAccountEmail(ctx)
	if err != nil {
		return err
	}
	if err := b.driver.EnsureUserHasServiceAccountRoles(ctx, user, b.uploadServiceAccount, []string{"roles/iam.serviceAccountTokenCreator"}); err != nil {
		return err
	}
	start := time.Now()
	for {
		if _, err := b.driver.SignBytes(ctx, []byte("testdata"), b.uploadServiceAccount); err != nil {
			if strings.Contains(err.Error(), "Permission 'iam.serviceAccounts.signBlob' denied on resource") {
				if time.Since(start) > 5*time.Minute {
					return errors.New("Could not wait for adding serviceAccountTokenCreator role to current user to take effect, please try again later")
				}
				pkg.SleepWithContext(ctx, 30*time.Second)
				continue
			}
			return err
		} else {
			break
		}
	}

	// 5. Setup Cloud Run Job
	serviceAccount := path.Base(b.cdServiceAccount)
	if err := b.driver.SetupJob(ctx, "defang-cd", serviceAccount, []types.Container{
		{
			Image:     "us-central1-docker.pkg.dev/defang-cd-idhk6xblr21o/defang-cd/gcpcd:test",
			Name:      ecs.CdContainerName,
			Cpus:      2.0,
			Memory:    2048_000_000, // 2G
			Essential: ptr.Bool(true),
			WorkDir:   "/app",
			// VolumesFrom: []string{ cdTaskName, },
			// DependsOn:  map[string]types.ContainerCondition{cdTaskName: "START"},
			// EntryPoint: []string{"node", "lib/index.js"},
		},
	}); err != nil {
		return err
	}

	b.setupDone = true
	return nil
}

func (b *ByocGcp) BootstrapList(ctx context.Context) ([]string, error) {
	// FIXME: implement
	return nil, client.ErrNotImplemented("GCP bootstrap list")
}

func (b *ByocGcp) AccountInfo(ctx context.Context) (client.AccountInfo, error) {
	projectId := os.Getenv("GCP_PROJECT_ID")
	if projectId == "" {
		return nil, errors.New("GCP_PROJECT_ID must be set for GCP projects")
	}
	email, err := b.driver.GetCurrentAccountEmail(ctx)
	if err != nil {
		return nil, err
	}
	return GcpAccountInfo{
		projectId: projectId,
		region:    b.driver.Region,
		email:     email,
	}, nil
}

type GcpAccountInfo struct {
	projectId string
	region    string
	email     string
}

func (g GcpAccountInfo) AccountID() string {
	return g.email
}

func (g GcpAccountInfo) Region() string {
	return g.region
}

func (g GcpAccountInfo) Details() string {
	return g.projectId
}

func (g GcpAccountInfo) Provider() client.ProviderID {
	return client.ProviderGCP
}

func (b *ByocGcp) BootstrapCommand(ctx context.Context, req client.BootstrapCommandRequest) (types.ETag, error) {
	if err := b.setUpCD(ctx); err != nil {
		return "", err
	}
	cmd := cdCommand{
		Project: req.Project,
		Command: []string{req.Command},
	}
	cdTaskId, err := b.runCdCommand(ctx, cmd) // TODO: make domain optional for defang cd
	if err != nil {
		return "", err
	}
	return cdTaskId, nil
}

type cdCommand struct {
	Project     string
	Command     []string
	EnvOverride map[string]string
	Mode        defangv1.DeploymentMode
}

func (b *ByocGcp) runCdCommand(ctx context.Context, cmd cdCommand) (string, error) {
	env := map[string]string{
		"PROJECT":                  cmd.Project,
		"PULUMI_BACKEND_URL":       `gs://` + b.bucket,
		"PULUMI_CONFIG_PASSPHRASE": pkg.Getenv("PULUMI_CONFIG_PASSPHRASE", "asdf"), // TODO: make customizable
		"REGION":                   b.driver.Region,
		"DOMAIN":                   cmd.Project + ".defang.dev", // FIXME: Use delegated domain
		"DEFANG_ORG":               "defang",
		"GCP_PROJECT":              b.driver.ProjectId,
		"STACK":                    "beta",
		"DEFANG_PREFIX":            "defang",
		"NO_COLOR":                 "true", // FIXME:  Remove later, for easier viewing in gcloud console for now
		"DEFANG_MODE":              strings.ToLower(cmd.Mode.String()),
	}

	for k, v := range cmd.EnvOverride {
		env[k] = v
	}

	execution, err := b.driver.Run(ctx, "defang-cd", env, cmd.Command...)
	if err != nil {
		return "", err
	}
	b.lastCdExecution = execution
	// fmt.Printf("CD Execution: %s\n", execution)
	return execution, nil
}

func (b *ByocGcp) CreateUploadURL(ctx context.Context, req *defangv1.UploadURLRequest) (*defangv1.UploadURLResponse, error) {
	if err := b.setUpCD(ctx); err != nil {
		return nil, err
	}

	url, err := b.driver.CreateUploadURL(ctx, b.bucket, req.Digest, b.uploadServiceAccount)
	if err != nil {
		if strings.Contains(err.Error(), "Permission 'iam.serviceAccounts.signBlob' denied on resource") {
			return nil, errors.New("Current user do not have 'iam.serviceAccounts.signBlob' permission, if it has been recently added, please wait for a few minutes and try again")
		}
		return nil, err
	}
	return &defangv1.UploadURLResponse{Url: url}, nil
}

func (b *ByocGcp) Deploy(ctx context.Context, req *defangv1.DeployRequest) (*defangv1.DeployResponse, error) {
	// If multiple Compose files were provided, req.Compose is the merged representation of all the files
	project, err := compose.LoadFromContent(ctx, req.Compose, "")
	if err != nil {
		return nil, err
	}

	// FIXME: Get cd image tag for the project

	if err := b.setUpCD(ctx); err != nil {
		return nil, err
	}

	etag := pkg.RandomID()
	var serviceInfos []*defangv1.ServiceInfo
	for _, service := range project.Services {
		serviceInfo := b.update(service, project.Name)
		serviceInfo.Etag = etag
		serviceInfos = append(serviceInfos, serviceInfo)
	}

	data, err := proto.Marshal(&defangv1.ProjectUpdate{
		// CdVersion: , // FIXME cd version support
		Compose:  req.Compose,
		Services: serviceInfos,
	})
	if err != nil {
		return nil, err
	}

	var payload string
	if len(data) < 1000 {
		payload = base64.StdEncoding.EncodeToString(data)
	} else {
		payloadUrl, err := b.driver.CreateUploadURL(ctx, b.bucket, etag, b.uploadServiceAccount)
		if err != nil {
			return nil, err
		}

		resp, err := http.Put(ctx, payloadUrl, "application/protobuf", bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("unexpected status code during upload: %s", resp.Status)
		}
		payload = http.RemoveQueryParam(payloadUrl)
	}

	cmd := cdCommand{
		Mode:        req.Mode,
		Project:     project.Name,
		Command:     []string{"up", payload},
		EnvOverride: map[string]string{"DEFANG_ETAG": etag},
	}

	if _, err := b.runCdCommand(ctx, cmd); err != nil {
		return nil, err
	}
	b.lastCdEtag = etag
	return &defangv1.DeployResponse{Etag: etag, Services: serviceInfos}, nil
}

func (b *ByocGcp) update(service composeTypes.ServiceConfig, projectName string) *defangv1.ServiceInfo {
	// TODO: Copied from DO provider, double check if more is needed
	si := &defangv1.ServiceInfo{
		Project: projectName,
		Service: &defangv1.Service{Name: service.Name},
	}

	for _, port := range service.Ports {
		mode := defangv1.Mode_INGRESS
		if port.Mode == compose.Mode_HOST {
			mode = defangv1.Mode_HOST
		}
		si.Service.Ports = append(si.Service.Ports, &defangv1.Port{
			Target: port.Target,
			Mode:   mode,
		})
	}

	si.Status = "UPDATE_QUEUED"
	si.State = defangv1.ServiceState_UPDATE_QUEUED
	if service.Build != nil {
		si.Status = "BUILD_QUEUED" // in SaaS, this gets overwritten by the ECS events for "kaniko"
		si.State = defangv1.ServiceState_BUILD_QUEUED
	}
	return si
}

func (b *ByocGcp) Subscribe(ctx context.Context, req *defangv1.SubscribeRequest) (client.ServerStream[defangv1.SubscribeResponse], error) {
	ss := NewSubscribeStream(ctx, b.driver)
	if req.Etag == b.lastCdEtag {
		if err := ss.AddJobExecutionUpdate(ctx, path.Base(b.lastCdExecution)); err != nil {
			return nil, err
		}
	}

	if err := ss.AddJobStatusUpdate(ctx, req.Project, req.Etag, req.Services); err != nil {
		return nil, err
	}

	if err := ss.AddServiceStatusUpdate(ctx, req.Project, req.Etag, req.Services); err != nil { // ALl services of the etag
		return nil, err
	}
	return ss, nil
}

func (b *ByocGcp) Follow(ctx context.Context, req *defangv1.TailRequest) (client.ServerStream[defangv1.TailResponse], error) {
	ls := NewLogStream(ctx, b.driver)
	if req.Etag == b.lastCdEtag {
		// Note: project is not supported in the CD logs yet, this send empty string
		if err := ls.AddJobLog(ctx, "", path.Base(b.lastCdExecution), nil, req.Since.AsTime()); err != nil {
			return nil, err
		}
	} else if req.Etag == b.lastCdExecution { // Execution ID passed in as etag: only for CD logs
		if err := ls.AddJobLog(ctx, "", path.Base(b.lastCdExecution), nil, req.Since.AsTime()); err != nil {
			return nil, err
		}
		return ls, nil
	}

	// FIXME: Support kaniko build logs
	if err := ls.AddJobLog(ctx, req.Project, "", req.Services, req.Since.AsTime()); err != nil {
		return nil, err
	}

	if err := ls.AddServiceLog(ctx, req.Project, req.Etag, req.Services, req.Since.AsTime()); err != nil {
		return nil, err
	}
	return ls, nil
}

func (b *ByocGcp) GetService(ctx context.Context, req *defangv1.GetRequest) (*defangv1.ServiceInfo, error) {
	// FIXME: implement
	return nil, client.ErrNotImplemented("GCP GetService")
}

func (b *ByocGcp) GetServices(ctx context.Context, req *defangv1.GetServicesRequest) (*defangv1.GetServicesResponse, error) {
	// FIXME: implement
	return nil, client.ErrNotImplemented("GCP GetServices")
}

// FUNCTIONS TO BE IMPLEMENTED BELOW ========================

func (b *ByocGcp) PrepareDomainDelegation(ctx context.Context, req client.PrepareDomainDelegationRequest) (*client.PrepareDomainDelegationResponse, error) {
	// FIXME: implement
	// return nil, client.ErrNotImplemented("GCP PrepareDomainDelegation")
	return &client.PrepareDomainDelegationResponse{
		NameServers: []string{"ns1.google.com", "ns2.google.com"},
	}, nil
}

func (b *ByocGcp) Delete(ctx context.Context, req *defangv1.DeleteRequest) (*defangv1.DeleteResponse, error) {
	// FIXME: implement
	return nil, client.ErrNotImplemented("GCP Delete")
}

func (b *ByocGcp) DeleteConfig(ctx context.Context, req *defangv1.Secrets) error {
	// FIXME: implement
	return client.ErrNotImplemented("GCP DeleteConfig")
}

func (b *ByocGcp) Destroy(ctx context.Context, req *defangv1.DestroyRequest) (types.ETag, error) {
	return b.BootstrapCommand(ctx, client.BootstrapCommandRequest{Project: req.Project, Command: "down"})
}

func (b *ByocGcp) ListConfig(ctx context.Context, req *defangv1.ListConfigsRequest) (*defangv1.Secrets, error) {
	// FIXME: implement
	return nil, client.ErrNotImplemented("GCP ListConfig")
}

func (b *ByocGcp) Query(ctx context.Context, req *defangv1.DebugRequest) error {
	// FIXME: implement
	return client.ErrNotImplemented("GCP Query")
}

func (b *ByocGcp) Preview(ctx context.Context, req *defangv1.DeployRequest) (*defangv1.DeployResponse, error) {
	// FIXME: implement
	return nil, client.ErrNotImplemented("GCP Preview")
}

func (b *ByocGcp) PutConfig(ctx context.Context, req *defangv1.PutConfigRequest) error {
	// FIXME: implement
	return client.ErrNotImplemented("GCP PutConfig")
}

func (b *ByocGcp) TearDown(ctx context.Context) error {
	// FIXME: implement
	return client.ErrNotImplemented("GCP TearDown")
}
