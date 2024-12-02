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

	"cloud.google.com/go/storage"
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
	"github.com/bufbuild/connect-go"
	composeTypes "github.com/compose-spec/compose-go/v2/types"
	"google.golang.org/api/googleapi"
	"google.golang.org/protobuf/proto"
)

var _ client.Provider = (*ByocGcp)(nil)

const (
	DefangCDProjectName = "defang-cd"
	UploadPrefix        = "uploads/"
)

var (
	DefaultCDTags    = map[string]string{"created-by": "defang"}
	PulumiVersion    = pkg.Getenv("DEFANG_PULUMI_VERSION", "3.136.1")
	DefangGcpCdImage = pkg.Getenv("DEFANG_GCP_CD_IMAGE", "edwardrf/gcpcd:test")

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

type ByocGcp struct {
	*byoc.ByocBaseClient

	driver *gcp.Gcp

	bucket               string
	cdServiceAccount     string
	setupDone            bool
	uploadServiceAccount string
	delegateDomainZone   string

	lastCdExecution string
	lastCdEtag      string
}

func NewByocProvider(ctx context.Context, tenantId types.TenantID) *ByocGcp {
	region := pkg.Getenv("GCP_LOCATION", "us-central1") // Defaults to us-central1 for lower price
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

	term.Infof("Setting up defang CD in GCP project %s, this could take a few minutes", b.driver.ProjectId)
	// 1. Enable required APIs
	apis := []string{
		"storage.googleapis.com",              // Cloud Storage API
		"artifactregistry.googleapis.com",     // Artifact Registry API
		"run.googleapis.com",                  // Cloud Run API
		"iam.googleapis.com",                  // IAM API
		"cloudresourcemanager.googleapis.com", // For service account and role management
		"compute.googleapis.com",              // For load balancer
		"dns.googleapis.com",                  // For DNS
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
	if err := b.driver.EnsureServiceAccountHasRoles(ctx, b.cdServiceAccount, []string{
		"roles/run.admin",                       // For creating and running cloudrun jobs and services (admin needed for `setIamPolicy` permission)
		"roles/iam.serviceAccountAdmin",         // For creating service accounts
		"roles/iam.serviceAccountUser",          // For impersonating service accounts
		"roles/artifactregistry.admin",          // For creating artifact registry
		"roles/compute.futureReservationViewer", // For `compute.regions.list` permission to avoid pulumi error message
		"roles/compute.loadBalancerAdmin",       // For creating load balancer and ssl certs
		"roles/compute.networkAdmin",            // For creating network
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
	//  4.2 Give current user the token creator role on the upload service account
	user, err := b.driver.GetCurrentAccountEmail(ctx)
	if err != nil {
		return err
	}
	if err := b.driver.EnsureUserHasServiceAccountRoles(ctx, user, b.uploadServiceAccount, []string{"roles/iam.serviceAccountTokenCreator"}); err != nil {
		return err
	}
	//  4.3 Wait until we can sign bytes with the upload service account
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
			Image:     DefangGcpCdImage,
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
		return nil, errors.New("No defang cd bucket found")
	}

	prefix := `.pulumi/stacks/` // TODO: should we filter on `projectName`?

	var stacks []string
	err = b.driver.IterateBucketObjects(ctx, bucketName, prefix, func(obj *storage.ObjectAttrs) error {
		stack, err := b.ParsePulumiStackObject(ctx, gcpObj{obj}, bucketName, prefix, b.driver.GetBucketObject)
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

	url, err := b.driver.CreateUploadURL(ctx, b.bucket, path.Join(UploadPrefix, req.Digest), b.uploadServiceAccount)
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
	projectNumber, err := b.driver.GetProjectNumber(ctx)
	if err != nil {
		return nil, err
	}
	for _, service := range project.Services {
		serviceInfo := b.update(service, project.Name, projectNumber)
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

func (b *ByocGcp) update(service composeTypes.ServiceConfig, projectName string, projectNumber int64) *defangv1.ServiceInfo {
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

		si.Endpoints = append(si.Endpoints, fmt.Sprintf("%v-%v-%v.%v.run.app", service.Name, projectName, projectNumber, b.driver.Region))
		si.Service.Ports = append(si.Service.Ports, &defangv1.Port{
			Target: port.Target,
			Mode:   mode,
		})
	}

	// TODO: Public FQDN

	si.Status = "UPDATE_QUEUED"
	si.State = defangv1.ServiceState_UPDATE_QUEUED
	if service.Build != nil {
		si.Status = "BUILD_QUEUED" // in SaaS, this gets overwritten by the ECS events for "kaniko"
		si.State = defangv1.ServiceState_BUILD_QUEUED
	}
	return si
}

func (b *ByocGcp) Subscribe(ctx context.Context, req *defangv1.SubscribeRequest) (client.ServerStream[defangv1.SubscribeResponse], error) {
	ss, err := NewSubscribeStream(ctx, b.driver)
	if err != nil {
		return nil, err
	}
	if req.Etag == b.lastCdEtag {
		ss.AddJobExecutionUpdate(path.Base(b.lastCdExecution))
	}
	ss.AddJobStatusUpdate(req.Project, req.Etag, req.Services)
	ss.AddServiceStatusUpdate(req.Project, req.Etag, req.Services)
	if err := ss.Start(); err != nil {
		return nil, err
	}
	return ss, nil
}

func (b *ByocGcp) Follow(ctx context.Context, req *defangv1.TailRequest) (client.ServerStream[defangv1.TailResponse], error) {
	ls, err := NewLogStream(ctx, b.driver)
	if err != nil {
		return nil, err
	}
	if req.Etag == b.lastCdEtag || req.Etag == b.lastCdExecution {
		ls.AddJobExecutionLog(path.Base(b.lastCdExecution), req.Since.AsTime()) // CD log
	}

	ls.AddJobLog(req.Project, req.Etag, req.Services, req.Since.AsTime())     // Kaniko logs
	ls.AddServiceLog(req.Project, req.Etag, req.Services, req.Since.AsTime()) // Service logs
	if err := ls.Start(); err != nil {
		return nil, err
	}
	return ls, nil
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
	projUpdate, err := b.getProjectUpdate(ctx, req.Project)
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

func (b *ByocGcp) PrepareDomainDelegation(ctx context.Context, req client.PrepareDomainDelegationRequest) (*client.PrepareDomainDelegationResponse, error) {
	term.Debugf("Preparing domain delegation for %s", req.DelegateDomain)
	// Ignore preview, always create the zone for the defang stack
	if zone, err := b.driver.EnsureDNSZoneExists(ctx, "defang", req.DelegateDomain, "defang delegate domain"); err != nil {
		return nil, err
	} else {
		b.delegateDomainZone = zone.Name
		term.Debugf("Zone %s created with nameservers %v", zone.Name, zone.NameServers)
		return &client.PrepareDomainDelegationResponse{
			NameServers: zone.NameServers,
		}, nil
	}
}

// FUNCTIONS TO BE IMPLEMENTED BELOW ========================

func (b *ByocGcp) Delete(ctx context.Context, req *defangv1.DeleteRequest) (*defangv1.DeleteResponse, error) {
	// FIXME: implement
	return nil, client.ErrNotImplemented("GCP Delete")
}

func (b *ByocGcp) DeleteConfig(ctx context.Context, req *defangv1.Secrets) error {
	// FIXME: implement
	return client.ErrNotImplemented("GCP DeleteConfig")
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
func GetGoogleAPIErrorDetail(detail interface{}, path string) string {
	if path == "" {
		value, ok := detail.(string)
		if ok {
			return value
		}
		return ""
	}
	dm, ok := detail.(map[string]interface{})
	if !ok {
		return ""
	}
	key, rest, _ := strings.Cut(path, ".")
	return GetGoogleAPIErrorDetail(dm[key], rest)
}

func (b *ByocGcp) getProjectUpdate(ctx context.Context, projectName string) (*defangv1.ProjectUpdate, error) {
	if projectName == "" {
		return nil, nil
	}
	bucketName, err := b.driver.GetBucketWithPrefix(ctx, "defang-cd")
	if err != nil {
		return nil, annotateGcpError(err)
	}
	if bucketName == "" {
		return nil, errors.New("No defang cd bucket found")
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
