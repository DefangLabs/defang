package gcp

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"iter"
	"os"
	"path"
	"strings"
	"time"

	"cloud.google.com/go/logging/apiv2/loggingpb"
	"cloud.google.com/go/storage"
	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/clouds/gcp"
	"github.com/DefangLabs/defang/src/pkg/dns"
	"github.com/DefangLabs/defang/src/pkg/http"
	"github.com/DefangLabs/defang/src/pkg/logs"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/bufbuild/connect-go"
	"go.yaml.in/yaml/v3"
	"google.golang.org/api/googleapi"
	auditpb "google.golang.org/genproto/googleapis/cloud/audit"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

var _ client.Provider = (*ByocGcp)(nil)

const (
	DefangCDProjectName            = "defang-cd"
	DefangUploadServiceAccountName = "defang-upload"
	UploadPrefix                   = "uploads/"
)

var (
	DefaultCDTags = map[string]string{"created-by": "defang"}

	//TODO: Create cd role with more fine-grained permissions
	// cdPermissions = []string{
	// 	"run.operations.get",
	// 	"run.operations.list",
	// 	"run.routes.get",
	// 	"run.routes.invoke",
	// 	"run.routes.list",
	// 	"run.services.create",
	// 	"run.services.delete",
	// 	"run.services.get",
	// 	"run.services.getIamPolicy",
	// 	"run.services.list",
	// 	"run.services.listEffectiveTags",
	// 	"run.services.listTagBindings",
	// 	"run.services.update",
	// 	"run.jobs.create",
	// 	"run.jobs.delete",
	// 	"run.jobs.get",
	// 	"run.jobs.getIamPolicy",
	// 	"run.jobs.list",
	// 	"run.jobs.listEffectiveTags",
	// 	"run.jobs.listTagBindings",
	// 	"run.jobs.run",
	// 	"run.jobs.runWithOverrides",
	// 	"run.jobs.update",
	// 	"compute.regions.list", // To avoid pulumi error message of unable to list regions
	// }
)

type CredentialsError struct {
	error
}

func (e CredentialsError) Unwrap() error {
	return e.error
}

type ByocGcp struct {
	*byoc.ByocBaseClient

	driver *gcp.Gcp

	bucket               string
	cdServiceAccount     string
	setupDone            bool
	uploadServiceAccount string
	delegateDomainZone   string

	cdExecution string
}

func NewByocProvider(ctx context.Context, tenantName types.TenantLabel, stack string) *ByocGcp {
	region := pkg.Getenv("GCP_LOCATION", "us-central1") // Defaults to us-central1 for lower price
	projectId := getGcpProjectID()
	b := &ByocGcp{driver: &gcp.Gcp{Region: region, ProjectId: projectId}}
	b.ByocBaseClient = byoc.NewByocBaseClient(tenantName, b, stack)
	return b
}

func getGcpProjectID() string {
	projectId, ok := os.LookupEnv("GCP_PROJECT_ID")
	if !ok {
		projectId = os.Getenv("CLOUDSDK_CORE_PROJECT")
	}
	return projectId
}

func (b *ByocGcp) SetUpCD(ctx context.Context) error {
	if b.setupDone {
		return nil
	}
	// TODO: Handle project creation flow

	term.Infof("Setting up defang CD in GCP project %s, this could take a few minutes", b.driver.ProjectId)
	// 1. Enable required APIs
	// TODO: enable minimum APIs needed for bootstrap the cd image, let CD enable the rest of the APIs
	apis := []string{
		"storage.googleapis.com",              // Cloud Storage API
		"artifactregistry.googleapis.com",     // Artifact Registry API
		"run.googleapis.com",                  // Cloud Run API
		"iam.googleapis.com",                  // IAM API
		"cloudresourcemanager.googleapis.com", // For service account and role management
		"cloudbuild.googleapis.com",           // For building images using cloud build
		"compute.googleapis.com",              // For load balancer
		"dns.googleapis.com",                  // For DNS
		"secretmanager.googleapis.com",        // For config/secrets
		"sqladmin.googleapis.com",             // For Cloud SQL
		"servicenetworking.googleapis.com",    // For VPC peering
		"redis.googleapis.com",                // For Redis
		"certificatemanager.googleapis.com",   // For SSL certs
		// "config.googleapis.com", // Infrastructure Manager API, for future CD stack
	}
	if err := b.driver.EnsureAPIsEnabled(ctx, apis...); err != nil {
		return annotateGcpError(err)
	}

	// 2. Setup cd bucket
	if bucket, err := b.driver.EnsureBucketExists(ctx, "defang-cd"); err != nil {
		return err
	} else {
		b.bucket = bucket
	}

	// 3. Setup CD Service Accounts and its permissions

	// TODO: Use cd permissions to give even less permissions to cd service account
	// if cdRole, err := b.driver.EnsureRoleExists(ctx, "defang_cd_role", "defang CD", "defang CD deployment role", cdPermissions); err != nil {
	// 	return err
	// }
	//   3.1 create CD Service Account
	if cdServiceAccount, err := b.driver.EnsureServiceAccountExists(ctx, "defang-cd", "defang CD", "Service account for defang CD"); err != nil {
		return err
	} else {
		b.cdServiceAccount = path.Base(cdServiceAccount)
	}
	//   3.2 Give CD service account roles needed
	if err := b.driver.EnsurePrincipalHasRoles(ctx, "serviceAccount:"+b.cdServiceAccount, []string{
		"roles/run.admin",                       // For creating and running cloudrun jobs and services (admin needed for `setIamPolicy` permission)
		"roles/iam.serviceAccountAdmin",         // For creating service accounts
		"roles/iam.serviceAccountUser",          // For impersonating service accounts
		"roles/artifactregistry.admin",          // For creating artifact registry
		"roles/compute.futureReservationViewer", // For `compute.regions.list` permission to avoid pulumi error message
		"roles/compute.loadBalancerAdmin",       // For creating load balancer and ssl certs
		"roles/compute.networkAdmin",            // For creating network
		"roles/compute.securityAdmin",           // For creating firewall rules
		"roles/dns.admin",                       // For creating DNS records
		"roles/cloudbuild.builds.editor",        // For building images using cloud build
		"roles/secretmanager.admin",             // For set permission to secrets
		"roles/resourcemanager.projectIamAdmin", // For assiging roles to service account used by service
		"roles/compute.instanceAdmin.v1",        // For creating compute instances
		"roles/cloudsql.admin",                  // For creating cloud sql instances
		"roles/redis.admin",                     // For creating redis instances/clusters
		"roles/certificatemanager.owner",        // For creating certificates
		"roles/serviceusage.serviceUsageAdmin",  // For allowing cd to Enable APIs
		"roles/datastore.owner",                 // For creating firestore database
		"roles/logging.logWriter",               // For allowing cloudbuild to write logs
	}); err != nil {
		return err
	}
	//   3.2 Give CD role access to CD bucket
	if err := b.driver.EnsurePrincipalHasBucketRoles(ctx, b.bucket, "serviceAccount:"+b.cdServiceAccount, []string{"roles/storage.admin"}); err != nil {
		return err
	}

	// 4 Setup service account for upload and give ability to create signed URLs using it to current user
	if uploadServiceAccount, err := b.driver.EnsureServiceAccountExists(ctx, DefangUploadServiceAccountName, "defang upload", "Service account for defang cli to generate pre-signed URL to upload artifacts"); err != nil {
		return err
	} else {
		b.uploadServiceAccount = path.Base(uploadServiceAccount)
	}
	//  4.1 Give upload service account access to cd bucket
	if err := b.driver.EnsurePrincipalHasBucketRoles(ctx, b.bucket, "serviceAccount:"+b.uploadServiceAccount, []string{"roles/storage.objectUser"}); err != nil {
		return err
	}
	//  4.2 Give current principal the token creator role on the upload service account
	principal, err := b.driver.GetCurrentPrincipal(ctx)
	if err != nil {
		return err
	}
	if err := b.driver.EnsurePrincipalHasServiceAccountRoles(ctx, principal, b.uploadServiceAccount, []string{"roles/iam.serviceAccountTokenCreator"}); err != nil {
		return err
	}
	//  4.3 Wait until we can sign bytes with the upload service account
	start := time.Now()
	for {
		if _, err := b.driver.SignBytes(ctx, []byte("testdata"), b.uploadServiceAccount); err != nil {
			if strings.Contains(err.Error(), "Permission 'iam.serviceAccounts.signBlob' denied on resource") {
				if time.Since(start) > 5*time.Minute {
					return errors.New("could not wait for adding serviceAccountTokenCreator role to current principal to take effect, please try again later")
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
	term.Debugf("Using CD image: %q", b.CDImage)

	b.setupDone = true
	return nil
}

type gcpObj struct{ obj *storage.ObjectAttrs }

func (o gcpObj) Name() string {
	return o.obj.Name
}

func (o gcpObj) Size() int64 {
	return o.obj.Size
}

func (b *ByocGcp) CdList(ctx context.Context, _allRegions bool) (iter.Seq[string], error) {
	bucketName, err := b.driver.GetBucketWithPrefix(ctx, "defang-cd")
	if err != nil {
		return nil, annotateGcpError(err)
	}
	if bucketName == "" {
		return nil, errors.New("no defang cd bucket found")
	}

	prefix := `.pulumi/stacks/` // TODO: should we filter on `projectName`?

	uploadSA := b.driver.GetServiceAccountEmail(DefangUploadServiceAccountName)
	term.Debug("Getting services from pulumi stacks bucket:", bucketName, prefix, uploadSA)
	objLoader := func(ctx context.Context, bucket, object string) ([]byte, error) {
		return b.driver.GetBucketObjectWithServiceAccount(ctx, bucket, object, uploadSA)
	}
	seq, err := b.driver.IterateBucketObjects(ctx, bucketName, prefix)
	if err != nil {
		return nil, annotateGcpError(err)
	}
	return func(yield func(string) bool) {
		for obj, err := range seq {
			if err != nil {
				term.Debugf("Error listing object in bucket %s: %v", bucketName, annotateGcpError(err))
				continue
			}
			stack, err := byoc.ParsePulumiStateFile(ctx, gcpObj{obj}, bucketName, objLoader)
			if err != nil {
				term.Debugf("Skipping %q in bucket %s: %v", obj.Name, bucketName, annotateGcpError(err))
				continue
			}
			if stack != nil {
				if !yield(stack.String()) {
					break
				}
			}
		}
	}, nil
}

func (b *ByocGcp) AccountInfo(ctx context.Context) (*client.AccountInfo, error) {
	projectId := getGcpProjectID()
	if projectId == "" {
		return nil, errors.New("GCP_PROJECT_ID or CLOUDSDK_CORE_PROJECT must be set for GCP projects; use 'gcloud projects list' to see available project ids")
	}

	// check whether the ADC is logged in by trying to get the current account email
	email, err := b.driver.GetCurrentPrincipal(ctx)
	if err != nil {
		// not logged in, get email from gcloud
		email, gcloudErr := GetUserEmail()
		if gcloudErr != nil {
			return nil, fmt.Errorf("failed to get GCP credentials for project: %q. %w", projectId, err)
		}

		credErr := fmt.Errorf("failed to get GCP credentials for user: %q project: %q. %w", email, projectId, err)
		return nil, &CredentialsError{credErr}
	}
	return &client.AccountInfo{
		AccountID: projectId,
		Region:    b.driver.Region,
		Details:   email,
		Provider:  client.ProviderGCP,
	}, nil
}

func (b *ByocGcp) CdCommand(ctx context.Context, req client.CdCommandRequest) (types.ETag, error) {
	if err := b.SetUpCD(ctx); err != nil {
		return "", err
	}
	etag := types.NewEtag()
	cmd := cdCommand{
		project:   req.Project,
		command:   []string{string(req.Command)},
		etag:      etag,
		statesUrl: req.StatesUrl,
		eventsUrl: req.EventsUrl,
	}
	err := b.runCdCommand(ctx, cmd) // TODO: make domain optional for defang cd
	if err != nil {
		return "", err
	}
	return etag, nil
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

type CloudBuildStep struct {
	Name       string   `yaml:"name,omitempty"`
	Entrypoint string   `yaml:"entrypoint,omitempty"`
	Args       []string `yaml:"args,omitempty"`
	Env        []string `yaml:"env,omitempty"`
}

func (b *ByocGcp) runCdCommand(ctx context.Context, cmd cdCommand) error {
	defangStateUrl := `gs://` + b.bucket
	pulumiBackendKey, pulumiBackendValue, err := byoc.GetPulumiBackend(defangStateUrl)
	if err != nil {
		return err
	}
	env := map[string]string{
		"DEFANG_DEBUG":             os.Getenv("DEFANG_DEBUG"), // TODO: use the global DoDebug flag
		"DEFANG_JSON":              os.Getenv("DEFANG_JSON"),
		"DEFANG_MODE":              strings.ToLower(cmd.mode.String()),
		"DEFANG_ORG":               string(b.TenantLabel),
		"DEFANG_PREFIX":            b.Prefix,
		"DEFANG_PULUMI_DEBUG":      os.Getenv("DEFANG_PULUMI_DEBUG"),
		"DEFANG_PULUMI_DIFF":       os.Getenv("DEFANG_PULUMI_DIFF"),
		"DEFANG_STATE_URL":         defangStateUrl,
		"GCP_PROJECT":              b.driver.ProjectId,
		"PROJECT":                  cmd.project,
		"PULUMI_CONFIG_PASSPHRASE": byoc.PulumiConfigPassphrase, // TODO: make secret
		"PULUMI_COPILOT":           "false",
		"PULUMI_SKIP_UPDATE_CHECK": "true",
		"REGION":                   b.driver.Region,
		"STACK":                    b.PulumiStack,
		pulumiBackendKey:           pulumiBackendValue, // TODO: make secret
	}

	if !term.StdoutCanColor() {
		env["NO_COLOR"] = "1"
	}

	if cmd.delegateDomain != "" {
		env["DOMAIN"] = cmd.delegateDomain
	} else {
		env["DOMAIN"] = "dummy.domain"
	}

	if cmd.statesUrl != "" {
		env["DEFANG_STATES_UPLOAD_URL"] = cmd.statesUrl
	}

	if cmd.eventsUrl != "" {
		env["DEFANG_EVENTS_UPLOAD_URL"] = cmd.eventsUrl
	}

	if cmd.etag != "" {
		env["DEFANG_ETAG"] = cmd.etag
	}

	if os.Getenv("DEFANG_PULUMI_DIR") != "" {
		debugEnv := []string{"REGION=" + b.driver.Region}
		if gcpProject := os.Getenv("GCP_PROJECT_ID"); gcpProject != "" {
			debugEnv = append(debugEnv, "GCP_PROJECT_ID="+gcpProject)
		}
		for k, v := range env {
			debugEnv = append(debugEnv, k+"="+v)
		}
		if err := byoc.DebugPulumiGolang(ctx, debugEnv, cmd.command...); err != nil {
			return err
		}
	}

	var envs []string
	for k, v := range env {
		envs = append(envs, fmt.Sprintf("%s=%s", k, v))
	}

	steps, err := yaml.Marshal([]CloudBuildStep{
		{
			Name: b.CDImage,
			Args: cmd.command,
			Env:  envs,
		},
	})
	if err != nil {
		return err
	}
	term.Debugf("Starting CD in cloudbuild at: %v", time.Now().Format(time.RFC3339))
	execution, err := b.driver.RunCloudBuild(ctx, gcp.CloudBuildArgs{
		Steps:          string(steps),
		ServiceAccount: &b.cdServiceAccount,
		Tags: []string{
			fmt.Sprintf("%v_%v_%v_%v", b.PulumiStack, cmd.project, "cd", cmd.etag), // For cd logs, consistent with cloud build tagging
			"defang-cd", // To indicate this is the actual cd service
		},
	})
	if err != nil {
		return err
	}
	b.cdExecution = execution

	return nil
}

func (b *ByocGcp) CreateUploadURL(ctx context.Context, req *defangv1.UploadURLRequest) (*defangv1.UploadURLResponse, error) {
	if err := b.SetUpCD(ctx); err != nil {
		return nil, err
	}

	url, err := b.driver.CreateUploadURL(ctx, b.bucket, path.Join(UploadPrefix, req.Digest), b.uploadServiceAccount)
	if err != nil {
		if strings.Contains(err.Error(), "Permission 'iam.serviceAccounts.signBlob' denied on resource") {
			return nil, errors.New("current user does not have 'iam.serviceAccounts.signBlob' permission. If it has been recently added, please wait a few minutes then try again")
		}
		return nil, err
	}
	return &defangv1.UploadURLResponse{Url: url}, nil
}
func (b *ByocGcp) Deploy(ctx context.Context, req *client.DeployRequest) (*defangv1.DeployResponse, error) {
	return b.deploy(ctx, req, "up")
}

func (b *ByocGcp) Preview(ctx context.Context, req *client.DeployRequest) (*defangv1.DeployResponse, error) {
	return b.deploy(ctx, req, "preview")
}

func (b *ByocGcp) GetDeploymentStatus(ctx context.Context) error {
	return b.driver.GetBuildStatus(ctx, b.cdExecution)
}

func (b *ByocGcp) deploy(ctx context.Context, req *client.DeployRequest, command string) (*defangv1.DeployResponse, error) {
	// If multiple Compose files were provided, req.Compose is the merged representation of all the files
	project, err := compose.LoadFromContent(ctx, req.Compose, "")
	if err != nil {
		return nil, err
	}

	// FIXME: Get cd image tag for the project

	if err := b.SetUpCD(ctx); err != nil {
		return nil, err
	}

	etag := types.NewEtag()
	serviceInfos, err := b.GetServiceInfos(ctx, project.Name, req.DelegateDomain, etag, project.Services)
	if err != nil {
		return nil, err
	}

	data, err := proto.Marshal(&defangv1.ProjectUpdate{
		CdVersion: b.CDImage,
		Compose:   req.Compose,
		Mode:      req.Mode,
		Services:  serviceInfos,
	})
	if err != nil {
		return nil, err
	}

	var payload string
	if len(data) < 1000 {
		payload = base64.StdEncoding.EncodeToString(data)
	} else {
		payloadUrl, err := b.driver.CreateUploadURL(ctx, b.bucket, path.Join(UploadPrefix, etag), b.uploadServiceAccount)
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
		// Only gs:// is supported in the payload as http get in gcpcd does not handle auth yet
		payload = strings.Replace(payload, "https://storage.googleapis.com/", "gs://", 1)
	}

	cdCmd := cdCommand{
		command:        []string{command, payload},
		delegateDomain: req.DelegateDomain,
		etag:           etag,
		mode:           req.Mode,
		project:        project.Name,
		statesUrl:      req.StatesUrl,
		eventsUrl:      req.EventsUrl,
	}
	if err := b.runCdCommand(ctx, cdCmd); err != nil {
		return nil, err
	}

	return &defangv1.DeployResponse{Etag: etag, Services: serviceInfos}, nil
}

func (b *ByocGcp) Subscribe(ctx context.Context, req *defangv1.SubscribeRequest) (client.ServerStream[defangv1.SubscribeResponse], error) {
	ignoreCdSuccess := func(entry *defangv1.SubscribeResponse) *defangv1.SubscribeResponse {
		if entry.Name != defangCD {
			return entry
		}
		return nil
	}
	subscribeStream, err := NewSubscribeStream(ctx, b.driver, true, req.Etag, req.Services, ignoreCdSuccess)
	if err != nil {
		return nil, err
	}
	subscribeStream.AddJobStatusUpdate(b.PulumiStack, req.Project, req.Etag, req.Services)
	subscribeStream.AddServiceStatusUpdate(b.PulumiStack, req.Project, req.Etag, req.Services)
	subscribeStream.StartFollow(time.Now())
	return subscribeStream, nil
}

func (b *ByocGcp) QueryLogs(ctx context.Context, req *defangv1.TailRequest) (client.ServerStream[defangv1.TailResponse], error) {
	return b.getLogStream(ctx, b.driver, req)
}

func (b *ByocGcp) getLogStream(ctx context.Context, gcpLogsClient GcpLogsClient, req *defangv1.TailRequest) (client.ServerStream[defangv1.TailResponse], error) {
	logStream, err := NewLogStream(ctx, gcpLogsClient, req.Services)
	if err != nil {
		return nil, err
	}

	if req.Since.IsValid() {
		logStream.AddSince(req.Since.AsTime())
	}
	if req.Until.IsValid() {
		logStream.AddUntil(req.Until.AsTime())
	}
	etag := req.Etag
	if logs.LogType(req.LogType).Has(logs.LogTypeBuild) {
		logStream.AddCloudBuildLog(b.PulumiStack, req.Project, etag, req.Services) // CD logs and CloudBuild logs
	}
	if logs.LogType(req.LogType).Has(logs.LogTypeRun) {
		logStream.AddServiceLog(b.PulumiStack, req.Project, etag, req.Services) // Service logs
	}
	logStream.AddFilter(req.Pattern)
	if req.Follow {
		logStream.StartFollow(req.Since.AsTime())
	} else if req.Since.IsValid() {
		logStream.StartHead(req.Limit)
	} else {
		logStream.StartTail(req.Limit)
	}
	return logStream, nil
}

func (b *ByocGcp) GetService(ctx context.Context, req *defangv1.GetRequest) (*defangv1.ServiceInfo, error) {
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

func (b *ByocGcp) GetServices(ctx context.Context, req *defangv1.GetServicesRequest) (*defangv1.GetServicesResponse, error) {
	projUpdate, err := b.GetProjectUpdate(ctx, req.Project)
	if err != nil {
		return nil, err
	}

	listServiceResp := defangv1.GetServicesResponse{}
	if projUpdate != nil {
		listServiceResp.Services = projUpdate.Services
		listServiceResp.Project = projUpdate.Project
	}

	return &listServiceResp, nil
}

type ConflictDelegateDomainError struct {
	NewDomain      string
	ConflictDomain string
	ConflictZone   string
	Err            error
}

func (e ConflictDelegateDomainError) Error() string {
	return e.Err.Error()
}

func (b *ByocGcp) PrepareDomainDelegation(ctx context.Context, req client.PrepareDomainDelegationRequest) (*client.PrepareDomainDelegationResponse, error) {
	term.Debugf("Preparing domain delegation for %s", req.DelegateDomain)
	name := "defang-" + dns.SafeLabel(req.DelegateDomain)
	if zone, err := b.driver.EnsureDNSZoneExists(ctx, name, req.DelegateDomain, "defang delegate domain"); err != nil {
		if apiErr := new(googleapi.Error); errors.As(err, &apiErr) {
			if strings.Contains(apiErr.Message, "Please verify ownership of") ||
				strings.Contains(apiErr.Message, "may be reserved or registered already") {
				var cde ConflictDelegateDomainError
				oldZone, err := b.driver.GetDNSZone(ctx, "defang") // Try if we can find the old defang delegate domain zone
				if err == nil && oldZone != nil {
					cde.ConflictDomain = oldZone.DnsName
					cde.ConflictZone = "defang"
				}
				cde.NewDomain = req.DelegateDomain
				cde.Err = apiErr
				return nil, cde
			}
		}
		return nil, err
	} else {
		b.delegateDomainZone = zone.Name
		term.Debugf("Zone %s created with nameservers %v", zone.Name, zone.NameServers)
		return &client.PrepareDomainDelegationResponse{
			NameServers: zone.NameServers,
		}, nil
	}
}

func (b *ByocGcp) DeleteConfig(ctx context.Context, req *defangv1.Secrets) error {
	for _, name := range req.Names {
		secretId := b.resourceName(req.Project, name)
		term.Debugf("Deleting secret %q", secretId)
		if err := b.driver.DeleteSecret(ctx, secretId); err != nil {
			return fmt.Errorf("failed to delete secret %q: %w", secretId, err)
		}
	}
	return nil
}

func (b *ByocGcp) ListConfig(ctx context.Context, req *defangv1.ListConfigsRequest) (*defangv1.Secrets, error) {
	prefix := b.resourceName(req.Project, "")
	secrets, err := b.driver.ListSecrets(ctx, prefix)
	if err != nil {
		if stat, ok := status.FromError(err); ok && stat.Code() == codes.PermissionDenied {
			if err := b.driver.EnsureAPIsEnabled(ctx, "secretmanager.googleapis.com"); err != nil {
				return nil, annotateGcpError(err)
			}
			secrets, err = b.driver.ListSecrets(ctx, prefix)
			if err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}
	return &defangv1.Secrets{Names: secrets}, nil
}

func (b *ByocGcp) PutConfig(ctx context.Context, req *defangv1.PutConfigRequest) error {
	if !pkg.IsValidSecretName(req.Name) {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid config name; must be alphanumeric or _, cannot start with a number: %q", req.Name))
	}
	secretId := b.resourceName(req.Project, req.Name)
	term.Debugf("Creating secret %q", secretId)

	if _, err := b.driver.CreateSecret(ctx, secretId); err != nil {
		if stat, ok := status.FromError(err); ok && stat.Code() == codes.PermissionDenied {
			if err := b.driver.EnsureAPIsEnabled(ctx, "secretmanager.googleapis.com"); err != nil {
				return annotateGcpError(err)
			}
			_, err = b.driver.CreateSecret(ctx, secretId)
		}
		if err != nil {
			if stat, ok := status.FromError(err); ok && stat.Code() == codes.AlreadyExists {
				term.Debugf("Secret %q already exists", secretId)
			} else {
				return fmt.Errorf("failed to create secret %q: %w", secretId, err)
			}
		}
	}
	term.Debugf("Adding a new secret version for %q", secretId)
	if _, err := b.driver.AddSecretVersion(ctx, secretId, []byte(req.Value)); err != nil {
		return fmt.Errorf("failed to add secret version for %q: %w", secretId, err)
	}
	if err := b.driver.CleanupOldVersionsExcept(ctx, secretId, 2); err != nil {
		return fmt.Errorf("failed to cleanup old versions for %q: %w", secretId, err)
	}
	return nil
}

func (b *ByocGcp) createDeploymentLogQuery(req *defangv1.DebugRequest) string {
	var since, until time.Time
	if req.Since.IsValid() {
		since = req.Since.AsTime()
	}
	if req.Until.IsValid() {
		until = req.Until.AsTime()
	}
	query := NewLogQuery(b.driver.GetProjectID())
	query.AddSince(since)
	query.AddUntil(until)

	// Logs
	query.AddCloudBuildLogQuery(b.PulumiStack, req.Project, req.Etag, req.Services)    // CloudBuild logs for CD and image builds
	query.AddServiceLogQuery(b.PulumiStack, req.Project, req.Etag, req.Services)       // Cloudrun service logs
	query.AddComputeEngineLogQuery(b.PulumiStack, req.Project, req.Etag, req.Services) // Compute Engine logs
	// Status Updates
	query.AddServiceStatusRequestUpdate(b.PulumiStack, req.Project, req.Etag, req.Services)
	query.AddServiceStatusReponseUpdate(b.PulumiStack, req.Project, req.Etag, req.Services)
	query.AddComputeEngineInstanceGroupInsertOrPatch(b.PulumiStack, req.Project, req.Etag, req.Services)

	return query.GetQuery()
}

func LogEntryToString(logEntry *loggingpb.LogEntry) (string, string, error) {
	result := ""
	emptySpace := strings.Repeat(" ", len(time.RFC3339)+1) // length of what a time stampe would be
	logTimestamp := emptySpace
	if logEntry.Timestamp != nil {
		logTimestamp = logEntry.Timestamp.AsTime().Local().Format(time.RFC3339) + " "
	}

	switch {
	case logEntry.GetJsonPayload() != nil:
		result += logTimestamp + logEntry.GetJsonPayload().String()
	case logEntry.GetProtoPayload() != nil:
		auditLog := &auditpb.AuditLog{}
		if err := logEntry.GetProtoPayload().UnmarshalTo(auditLog); err != nil {
			return logTimestamp, "", err
		}

		// we do not know the reason for no status events we will log it here (update in future)
		if auditLog.Status == nil {
			return logTimestamp, "<No Status>", nil
		}
		return logTimestamp, auditLog.Status.Message, nil
	case logEntry.GetTextPayload() != "":
		return logTimestamp, logEntry.GetTextPayload(), nil
	}

	return logTimestamp, result, nil
}

func LogEntriesToString(logEntries []*loggingpb.LogEntry) string {
	var result strings.Builder
	for _, logEntry := range logEntries {
		logTimestamp, msg, err := LogEntryToString(logEntry)
		if err != nil {
			continue
		}
		result.WriteString(logTimestamp)
		result.WriteByte(' ')
		result.WriteString(msg)
		result.WriteByte('\n')
	}
	return result.String()
}

func (b *ByocGcp) query(ctx context.Context, query string) ([]*loggingpb.LogEntry, error) {
	term.Debugf("Querying logs with filter: \n %s", query)
	var entries []*loggingpb.LogEntry
	lister, err := b.driver.ListLogEntries(ctx, query, gcp.OrderAscending)
	if err != nil {
		return nil, err
	}

	for {
		entry, err := lister.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

func (b *ByocGcp) QueryForDebug(ctx context.Context, req *defangv1.DebugRequest) error {
	logEntries, err := b.query(ctx, b.createDeploymentLogQuery(req))
	if err != nil {
		return annotateGcpError(err)
	}
	req.Logs = LogEntriesToString(logEntries)
	term.Debug(req.Logs)

	return nil
}

// FUNCTIONS TO BE IMPLEMENTED BELOW ========================

func (b *ByocGcp) TearDownCD(ctx context.Context) error {
	// FIXME: implement
	return client.ErrNotImplemented("GCP TearDown")
}

// Utility functions

// The default googleapi.Error is too verbose, only display the message if it exists
type briefGcpError struct {
	err *googleapi.Error
}

func (e briefGcpError) Error() string {
	if e.err.Message != "" {
		return e.err.Message
	}
	return e.err.Error()
}

func annotateGcpError(err error) error {
	gerr := new(googleapi.Error)
	if errors.As(err, &gerr) {
		// Check if this is a credential/ADC-related error
		if isADCRefreshNeeded(gerr) {
			return &CredentialsError{
				error: briefGcpError{err: gerr},
			}
		}
		return briefGcpError{err: gerr}
	}
	return err
}

// isADCRefreshNeeded checks if the error indicates that Application Default Credentials need to be refreshed
func isADCRefreshNeeded(gerr *googleapi.Error) bool {
	// Check for 403 Forbidden errors related to project access
	if gerr.Code != 403 {
		return false
	}

	// Check error message for common patterns
	msg := strings.ToLower(gerr.Message)
	if strings.Contains(msg, "has been deleted") ||
		strings.Contains(msg, "project") && strings.Contains(msg, "deleted") {
		return true
	}

	// Check error details for USER_PROJECT_DENIED or similar reasons
	for _, detail := range gerr.Details {
		if detailMap, ok := detail.(map[string]interface{}); ok {
			if reason, ok := detailMap["reason"].(string); ok {
				if reason == "USER_PROJECT_DENIED" {
					return true
				}
			}
		}
	}

	return false
}

// Used to get nested values from the detail of a googleapi.Error
func GetGoogleAPIErrorDetail(detail any, path string) string {
	if path == "" {
		value, ok := detail.(string)
		if ok {
			return value
		}
		return ""
	}
	dm, ok := detail.(map[string]any)
	if !ok {
		return ""
	}
	key, rest, _ := strings.Cut(path, ".")
	return GetGoogleAPIErrorDetail(dm[key], rest)
}

func (b *ByocGcp) GetProjectUpdate(ctx context.Context, projectName string) (*defangv1.ProjectUpdate, error) {
	if projectName == "" {
		return nil, nil
	}
	bucketName, err := b.driver.GetBucketWithPrefix(ctx, "defang-cd")
	if err != nil {
		return nil, annotateGcpError(err)
	}
	if bucketName == "" {
		return nil, errors.New("no defang cd bucket found")
	}

	path := b.GetProjectUpdatePath(projectName)

	// Current user might not have object viewer access to the bucket, use the upload service account to get the object
	uploadSA := b.driver.GetServiceAccountEmail(DefangUploadServiceAccountName)
	term.Debug("Getting services from bucket:", bucketName, path, uploadSA)
	pbBytes, err := b.driver.GetBucketObjectWithServiceAccount(ctx, bucketName, path, uploadSA)
	if err != nil {
		return nil, fmt.Errorf("failed to get project bucket object, try bootstrap the project with a deployment: %w", err)
	}

	projUpdate := defangv1.ProjectUpdate{}
	if err := proto.Unmarshal(pbBytes, &projUpdate); err != nil {
		return nil, err
	}

	return &projUpdate, nil
}

func (b *ByocGcp) resourceName(projectName, name string) string {
	pkg.Ensure(projectName != "", "ProjectName not set")
	var parts []string
	if b.Prefix != "" {
		parts = []string{b.Prefix}
	}
	return strings.Join(append(parts, projectName, b.PulumiStack, name), "_") // same as fullDefangResourceName in gcpcd/up.go
}

func (*ByocGcp) GetPrivateDomain(projectName string) string {
	// Apparently GCP does not support private DNS zones with arbitrary domain names
	return "google.internal"
}
