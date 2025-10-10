package gcp

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"

	"cloud.google.com/go/logging/apiv2/loggingpb"
	run "cloud.google.com/go/run/apiv2"
	"cloud.google.com/go/run/apiv2/runpb"
	"cloud.google.com/go/storage"
	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws/ecs"
	"github.com/DefangLabs/defang/src/pkg/clouds/gcp"
	"github.com/DefangLabs/defang/src/pkg/dns"
	"github.com/DefangLabs/defang/src/pkg/http"
	"github.com/DefangLabs/defang/src/pkg/logs"
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/aws/smithy-go/ptr"
	"github.com/bufbuild/connect-go"
	"google.golang.org/api/googleapi"
	auditpb "google.golang.org/genproto/googleapis/cloud/audit"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

var _ client.Provider = (*ByocGcp)(nil)

const (
	DefangCDProjectName = "defang-cd"
	UploadPrefix        = "uploads/"
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
	cdEtag      string
}

func NewByocProvider(ctx context.Context, tenantName types.TenantName) *ByocGcp {
	region := pkg.Getenv("GCP_LOCATION", "us-central1") // Defaults to us-central1 for lower price
	projectId := getGcpProjectID()
	b := &ByocGcp{driver: &gcp.Gcp{Region: region, ProjectId: projectId}}
	b.ByocBaseClient = byoc.NewByocBaseClient(ctx, tenantName, b)
	return b
}

func getGcpProjectID() string {
	projectId, ok := os.LookupEnv("GCP_PROJECT_ID")
	if !ok {
		projectId = os.Getenv("CLOUDSDK_CORE_PROJECT")
	}
	return projectId
}

func (b *ByocGcp) setUpCD(ctx context.Context) error {
	if b.setupDone {
		return nil
	}
	// TODO: Handle organizations and Project Creation

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
	if err := b.driver.EnsureServiceAccountHasRoles(ctx, "serviceAccount:"+b.cdServiceAccount, []string{
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
	}); err != nil {
		return err
	}
	//   3.2 Give CD role access to CD bucket
	if err := b.driver.EnsureServiceAccountHasBucketRoles(ctx, b.bucket, b.cdServiceAccount, []string{"roles/storage.admin"}); err != nil {
		return err
	}

	// 4 Setup service account for upload and give ability to create signed URLs using it to current user
	if uploadServiceAccount, err := b.driver.EnsureServiceAccountExists(ctx, "defang-upload", "defang upload", "Service account for defang cli to generate pre-signed URL to upload artifacts"); err != nil {
		return err
	} else {
		b.uploadServiceAccount = path.Base(uploadServiceAccount)
	}
	//  4.1 Give upload service account access to cd bucket
	if err := b.driver.EnsureServiceAccountHasBucketRoles(ctx, b.bucket, b.uploadServiceAccount, []string{"roles/storage.objectUser"}); err != nil {
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

	serviceAccount := path.Base(b.cdServiceAccount)
	if err := b.driver.SetupJob(ctx, "defang-cd", serviceAccount, []types.Container{
		{
			Image:     b.CDImage,
			Name:      ecs.CdContainerName,
			Cpus:      2.0,
			Memory:    2048_000_000, // 2G
			Essential: ptr.Bool(true),
			WorkDir:   "/app",
		},
	}); err != nil {
		return err
	}

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

func (b *ByocGcp) BootstrapList(ctx context.Context) ([]string, error) {
	bucketName, err := b.driver.GetBucketWithPrefix(ctx, "defang-cd")
	if err != nil {
		return nil, annotateGcpError(err)
	}
	if bucketName == "" {
		return nil, errors.New("no defang cd bucket found")
	}

	prefix := `.pulumi/stacks/` // TODO: should we filter on `projectName`?

	var stacks []string
	err = b.driver.IterateBucketObjects(ctx, bucketName, prefix, func(obj *storage.ObjectAttrs) error {
		stack, err := byoc.ParsePulumiStackObject(ctx, gcpObj{obj}, bucketName, prefix, b.driver.GetBucketObject)
		if err != nil {
			return err
		}
		if stack != "" {
			stacks = append(stacks, stack)
		}
		return nil
	})
	if err != nil {
		return nil, annotateGcpError(err)
	}
	return stacks, nil
}

func (b *ByocGcp) AccountInfo(ctx context.Context) (*client.AccountInfo, error) {
	projectId := getGcpProjectID()
	if projectId == "" {
		return nil, errors.New("GCP_PROJECT_ID or CLOUDSDK_CORE_PROJECT must be set for GCP projects")
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
	Project        string
	Command        []string
	EnvOverride    map[string]string
	Mode           modes.Mode
	DelegateDomain string
}

func (b *ByocGcp) runCdCommand(ctx context.Context, cmd cdCommand) (string, error) {
	defangStateUrl := `gs://` + b.bucket
	pulumiBackendKey, pulumiBackendValue, err := byoc.GetPulumiBackend(defangStateUrl)
	if err != nil {
		return "", err
	}
	env := map[string]string{
		"DEFANG_DEBUG":             os.Getenv("DEFANG_DEBUG"), // TODO: use the global DoDebug flag
		"DEFANG_JSON":              os.Getenv("DEFANG_JSON"),
		"DEFANG_MODE":              strings.ToLower(cmd.Mode.String()),
		"DEFANG_ORG":               "defang",
		"DEFANG_PREFIX":            byoc.DefangPrefix,
		"DEFANG_STATE_URL":         defangStateUrl,
		"GCP_PROJECT":              b.driver.ProjectId,
		"PROJECT":                  cmd.Project,
		pulumiBackendKey:           pulumiBackendValue,          // TODO: make secret
		"PULUMI_CONFIG_PASSPHRASE": byoc.PulumiConfigPassphrase, // TODO: make secret
		"PULUMI_COPILOT":           "false",
		"PULUMI_SKIP_UPDATE_CHECK": "true",
		"REGION":                   b.driver.Region,
		"STACK":                    b.PulumiStack,
	}

	if !term.StdoutCanColor() {
		env["NO_COLOR"] = "1"
	}

	if cmd.DelegateDomain != "" {
		env["DOMAIN"] = b.GetProjectDomain(cmd.Project, cmd.DelegateDomain)
	} else {
		env["DOMAIN"] = "dummy.domain"
	}

	for k, v := range cmd.EnvOverride {
		env[k] = v
	}

	if os.Getenv("DEFANG_PULUMI_DIR") != "" {
		debugEnv := []string{"REGION=" + b.driver.Region}
		if gcpProject := os.Getenv("GCP_PROJECT_ID"); gcpProject != "" {
			debugEnv = append(debugEnv, "GCP_PROJECT_ID="+gcpProject)
		}
		for k, v := range env {
			debugEnv = append(debugEnv, k+"="+v)
		}
		if err := byoc.DebugPulumiGolang(ctx, debugEnv, cmd.Command...); err != nil {
			return "", err
		}
	}

	execution, err := b.driver.Run(ctx, gcp.JobNameCD, env, cmd.Command...)
	if err != nil {
		return "", err
	}
	b.cdExecution = execution
	// fmt.Printf("CD Execution: %s\n", execution)
	return execution, nil
}

func (b *ByocGcp) CreateUploadURL(ctx context.Context, req *defangv1.UploadURLRequest) (*defangv1.UploadURLResponse, error) {
	if err := b.setUpCD(ctx); err != nil {
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
func (b *ByocGcp) Deploy(ctx context.Context, req *defangv1.DeployRequest) (*defangv1.DeployResponse, error) {
	return b.deploy(ctx, req, "up")
}

func (b *ByocGcp) Preview(ctx context.Context, req *defangv1.DeployRequest) (*defangv1.DeployResponse, error) {
	return b.deploy(ctx, req, "preview")
}

func (b *ByocGcp) GetDeploymentStatus(ctx context.Context) error {
	client, err := run.NewExecutionsClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	execution, err := client.GetExecution(ctx, &runpb.GetExecutionRequest{Name: b.cdExecution})
	if err != nil {
		return err
	}

	var completionTime = execution.GetCompletionTime()
	if completionTime != nil {
		// cd is done
		var failedTasks = execution.GetFailedCount()
		if failedTasks > 0 {
			var msgs []string
			for _, condition := range execution.GetConditions() {
				if condition.GetType() == "Completed" && condition.GetMessage() != "" {
					msgs = append(msgs, condition.GetMessage())
				}
			}

			return cliClient.ErrDeploymentFailed{Message: strings.Join(msgs, ",")}
		}

		// completed successfully
		return io.EOF
	}

	// still running
	return nil
}

func (b *ByocGcp) deploy(ctx context.Context, req *defangv1.DeployRequest, command string) (*defangv1.DeployResponse, error) {
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

	cmd := cdCommand{
		Mode:           modes.Mode(req.Mode),
		Project:        project.Name,
		Command:        []string{command, payload},
		EnvOverride:    map[string]string{"DEFANG_ETAG": etag},
		DelegateDomain: req.DelegateDomain,
	}

	execution, err := b.runCdCommand(ctx, cmd)
	if err != nil {
		return nil, err
	}

	b.cdEtag = etag
	if command == "preview" {
		etag = execution // Only wait for the preview command cd job to finish
	}
	return &defangv1.DeployResponse{Etag: etag, Services: serviceInfos}, nil
}

func (b *ByocGcp) GetStackName() string {
	return b.PulumiStack
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
	if req.Etag == b.cdEtag {
		subscribeStream.AddJobExecutionUpdate(path.Base(b.cdExecution))
	}

	// TODO: update stack (1st param) to b.PulumiStack
	subscribeStream.AddJobStatusUpdate("", req.Project, req.Etag, req.Services)
	subscribeStream.AddServiceStatusUpdate("", req.Project, req.Etag, req.Services)
	subscribeStream.Start(time.Now())
	return subscribeStream, nil
}

func (b *ByocGcp) QueryLogs(ctx context.Context, req *defangv1.TailRequest) (client.ServerStream[defangv1.TailResponse], error) {
	if b.cdExecution != "" && req.Etag == b.cdExecution { // Only follow CD log, we need to subscribe to cd activities to detect when the job is done
		subscribeStream, err := NewSubscribeStream(ctx, b.driver, true, req.Etag, req.Services)
		if err != nil {
			return nil, err
		}
		subscribeStream.AddJobExecutionUpdate(path.Base(b.cdExecution))
		var since time.Time
		if req.Since.IsValid() {
			since = req.Since.AsTime()
		}
		subscribeStream.Start(since)

		var cancel context.CancelCauseFunc
		ctx, cancel = context.WithCancelCause(ctx)
		go func() {
			defer subscribeStream.Close()
			for subscribeStream.Receive() {
				msg := subscribeStream.Msg()
				if msg.State == defangv1.ServiceState_BUILD_FAILED || msg.State == defangv1.ServiceState_DEPLOYMENT_FAILED {
					pkg.SleepWithContext(ctx, 3*time.Second) // Make sure the logs are flushed, gcp logs has a longer delay, thus 3s
					cancel(fmt.Errorf("CD job failed %s", msg.Status))
					return
				}
				if msg.State == defangv1.ServiceState_DEPLOYMENT_COMPLETED {
					pkg.SleepWithContext(ctx, 3*time.Second) // Make sure the logs are flushed, gcp logs has a longer delay, thus 3s
					cancel(io.EOF)
					return
				}
			}
			cancel(subscribeStream.Err())
		}()
	}

	logStream, err := NewLogStream(ctx, b.driver, req.Services)
	if err != nil {
		return nil, err
	}

	startTime := time.Now()
	if req.Since.IsValid() {
		startTime = req.Since.AsTime()
	}
	var endTime time.Time
	if req.Until.IsValid() {
		endTime = req.Until.AsTime()
	}
	if req.Since.IsValid() || req.Etag != "" {
		execName := path.Base(b.cdExecution)
		if execName == "." {
			execName = ""
		}
		etag := req.Etag
		if etag == b.cdExecution { // Do not pass the cd execution name as etag
			etag = ""
		}
		if logs.LogType(req.LogType).Has(logs.LogTypeBuild) {
			logStream.AddJobExecutionLog(execName) // CD log when there is an execution name
			// TODO: update stack (1st param) to b.PulumiStack
			logStream.AddJobLog("", req.Project, etag, req.Services)        // Kaniko or CD logs when there is no execution name
			logStream.AddCloudBuildLog("", req.Project, etag, req.Services) // CloudBuild logs
		}
		if logs.LogType(req.LogType).Has(logs.LogTypeRun) {
			// TODO: update stack (1st param) to b.PulumiStack
			logStream.AddServiceLog("", req.Project, etag, req.Services) // Service logs
		}
		logStream.AddSince(startTime)
		logStream.AddUntil(endTime)
		logStream.AddFilter(req.Pattern)
	}
	logStream.Start(startTime)
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

func (b *ByocGcp) Destroy(ctx context.Context, req *defangv1.DestroyRequest) (types.ETag, error) {
	return b.BootstrapCommand(ctx, client.BootstrapCommandRequest{Project: req.Project, Command: "down"})
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
		secretId := b.StackName(req.Project, name)
		term.Debugf("Deleting secret %q", secretId)
		if err := b.driver.DeleteSecret(ctx, secretId); err != nil {
			return fmt.Errorf("failed to delete secret %q: %w", secretId, err)
		}
	}
	return nil
}

func (b *ByocGcp) ListConfig(ctx context.Context, req *defangv1.ListConfigsRequest) (*defangv1.Secrets, error) {
	prefix := b.StackName(req.Project, "")
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
	secretId := b.StackName(req.Project, req.Name)
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
	query := NewLogQuery(b.driver.ProjectId)
	if b.cdExecution != "" {
		query.AddJobExecutionQuery(path.Base(b.cdExecution))
	}

	// Logs TODO: update stack (1st param) to b.PulumiStack
	query.AddJobLogQuery("", req.Project, req.Etag, req.Services)        // Kaniko OR CD logs
	query.AddServiceLogQuery("", req.Project, req.Etag, req.Services)    // Cloudrun service logs
	query.AddCloudBuildLogQuery("", req.Project, req.Etag, req.Services) // CloudBuild logs
	query.AddSince(since)
	query.AddUntil(until)

	// Service status updates TODO: update stack (1st param) to b.PulumiStack
	query.AddJobStatusUpdateRequestQuery("", req.Project, req.Etag, req.Services)
	query.AddJobStatusUpdateResponseQuery("", req.Project, req.Etag, req.Services)
	query.AddServiceStatusRequestUpdate("", req.Project, req.Etag, req.Services)
	query.AddServiceStatusReponseUpdate("", req.Project, req.Etag, req.Services)

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
	result := ""
	for _, logEntry := range logEntries {
		logTimestamp, msg, err := LogEntryToString(logEntry)
		if err != nil {
			continue
		}
		result += logTimestamp + " " + msg + "\n"
	}
	return result
}

func (b *ByocGcp) query(ctx context.Context, query string) ([]*loggingpb.LogEntry, error) {
	term.Debugf("Querying logs with filter: \n %s", query)
	var entries []*loggingpb.LogEntry
	lister, err := b.driver.ListLogEntries(ctx, query)
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

func (b *ByocGcp) Delete(ctx context.Context, req *defangv1.DeleteRequest) (*defangv1.DeleteResponse, error) {
	// FIXME: implement
	return nil, client.ErrNotImplemented("GCP Delete")
}

func (b *ByocGcp) TearDown(ctx context.Context) error {
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
		return briefGcpError{err: gerr}
	}
	return err
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

	// Path to the state file, Defined at: https://github.com/DefangLabs/defang-mvp/blob/main/pulumi/cd/byoc/aws/index.ts#L89
	path := fmt.Sprintf("projects/%s/%s/project.pb", projectName, b.PulumiStack)
	term.Debug("Getting services from bucket:", bucketName, path)
	pbBytes, err := b.driver.GetBucketObject(ctx, bucketName, path)
	if err != nil {
		return nil, err
	}

	projUpdate := defangv1.ProjectUpdate{}
	if err := proto.Unmarshal(pbBytes, &projUpdate); err != nil {
		return nil, err
	}

	return &projUpdate, nil
}

func (b *ByocGcp) StackName(projectName, name string) string {
	pkg.Ensure(projectName != "", "ProjectName not set")
	return fmt.Sprintf("%s_%s_%s_%s", byoc.DefangPrefix, projectName, b.PulumiStack, name)
}

func (b *ByocGcp) ServicePublicDNS(name string, projectName string) string {
	return dns.SafeLabel(name) + "." + b.PulumiStack + "." + dns.SafeLabel(projectName) + "." + dns.SafeLabel(b.TenantName) + ".defang.app"
}
