package do

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/digitalocean/godo"
	"github.com/muesli/termenv"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc"
	awsbyoc "github.com/DefangLabs/defang/src/pkg/cli/client/byoc/aws"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/logs"

	"github.com/DefangLabs/defang/src/pkg/clouds/aws"
	"github.com/DefangLabs/defang/src/pkg/clouds/do"
	"github.com/DefangLabs/defang/src/pkg/clouds/do/appPlatform"
	"github.com/DefangLabs/defang/src/pkg/clouds/do/region"
	"github.com/DefangLabs/defang/src/pkg/http"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	ansiCyan      = "\033[36m"
	ansiReset     = "\033[0m"
	DEFANG        = "defang"
	RFC3339Micro  = "2006-01-02T15:04:05.000000Z07:00"
	replaceString = ansiCyan + "$0" + ansiReset
)

var (
	colorKeyRegex = regexp.MustCompile(`"(?:\\["\\/bfnrt]|[^\x00-\x1f"\\]|\\u[0-9a-fA-F]{4})*"\s*:|[^\x00-\x20"=&?]+=`)
)

type ByocDo struct {
	*byoc.ByocBaseClient

	buildRepo      string
	client         *godo.Client
	driver         *appPlatform.DoApp
	cdAppID        string
	cdDeploymentID string
	cdEtag         types.ETag
	// cdStart      time.Time
}

var _ client.Provider = (*ByocDo)(nil)

func NewByocProvider(ctx context.Context, tenantName types.TenantName) *ByocDo {
	doRegion := do.Region(os.Getenv("REGION"))
	if doRegion == "" {
		doRegion = region.SFO3 // TODO: change default
	}

	client := appPlatform.NewClient(ctx)

	b := &ByocDo{
		client: client,
		driver: appPlatform.New(doRegion),
	}
	b.ByocBaseClient = byoc.NewByocBaseClient(ctx, tenantName, b)
	return b
}

func (b *ByocDo) GetProjectUpdate(ctx context.Context, projectName string) (*defangv1.ProjectUpdate, error) {
	s3client, err := b.driver.CreateS3Client()
	if err != nil {
		return nil, err
	}

	bucketName, err := b.driver.GetBucketName(ctx, s3client)
	if err != nil {
		return nil, err
	}

	if bucketName == "" {
		// bucket is not created yet; return empty update in that case
		return nil, nil // no services yet
	}

	path := fmt.Sprintf("projects/%s/%s/project.pb", projectName, b.PulumiStack)
	getObjectOutput, err := s3client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: &bucketName,
		Key:    &path,
	})

	if err != nil {
		if aws.IsS3NoSuchKeyError(err) {
			term.Debug("s3.GetObject:", err)
			return nil, nil // no services yet
		}
		return nil, awsbyoc.AnnotateAwsError(err)
	}
	defer getObjectOutput.Body.Close()
	pbBytes, err := io.ReadAll(getObjectOutput.Body)
	if err != nil {
		return nil, err
	}

	// TODO: this is to handle older deployment which may have been stored erroneously. Remove in future
	if decodedBuffer, err := base64.StdEncoding.DecodeString(string(pbBytes)); err == nil {
		pbBytes = decodedBuffer
	}

	projUpdate := defangv1.ProjectUpdate{}
	if err := proto.Unmarshal(pbBytes, &projUpdate); err != nil {
		return nil, err
	}

	return &projUpdate, nil
}

func (b *ByocDo) Preview(ctx context.Context, req *defangv1.DeployRequest) (*defangv1.DeployResponse, error) {
	return b.deploy(ctx, req, "preview")
}

func (b *ByocDo) Deploy(ctx context.Context, req *defangv1.DeployRequest) (*defangv1.DeployResponse, error) {
	return b.deploy(ctx, req, "up")
}

func (b *ByocDo) deploy(ctx context.Context, req *defangv1.DeployRequest, cmd string) (*defangv1.DeployResponse, error) {
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
		Mode:      req.Mode,
		Services:  serviceInfos,
	})
	if err != nil {
		return nil, err
	}

	payloadUrl, err := b.driver.CreateUploadURL(ctx, etag)
	if err != nil {
		return nil, err
	}

	// Do an HTTP PUT to the generated URL
	resp, err := http.Put(ctx, payloadUrl, "application/protobuf", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("unexpected status code during upload: %s", resp.Status)
	}

	// FIXME: find the bucket and key name
	parsedUrl, err := url.Parse(payloadUrl)
	if err != nil {
		return nil, err
	}
	payloadKey := strings.TrimLeft(parsedUrl.Path, "/"+b.driver.BucketName+"/")
	payloadString, err := b.driver.CreateS3DownloadUrl(ctx, payloadKey)
	if err != nil {
		return nil, err
	}

	_, err = b.runCdCommand(ctx, project.Name, req.DelegateDomain, cmd, payloadString)
	if err != nil {
		return nil, err
	}

	b.cdEtag = etag
	return &defangv1.DeployResponse{
		Services: serviceInfos,
		Etag:     etag,
	}, nil
}

func (b *ByocDo) GetDeploymentStatus(ctx context.Context) error {
	deploymentInfo, _, err := b.client.Apps.GetDeployment(ctx, b.cdAppID, b.cdDeploymentID)
	if err != nil {
		return err
	}

	switch deploymentInfo.GetPhase() {
	default:
		return nil // pending
	case godo.DeploymentPhase_Active:
		return io.EOF
	case godo.DeploymentPhase_Error, godo.DeploymentPhase_Canceled:
		return client.ErrDeploymentFailed{Message: deploymentInfo.Cause}
	}
}

func (b *ByocDo) BootstrapCommand(ctx context.Context, req client.BootstrapCommandRequest) (string, error) {
	if err := b.setUp(ctx); err != nil {
		return "", err
	}

	_, err := b.runCdCommand(ctx, req.Project, "dummy.domain", req.Command)
	if err != nil {
		return "", err
	}

	etag := pkg.RandomID()
	b.cdEtag = etag
	return etag, nil
}

func (b *ByocDo) BootstrapList(ctx context.Context) ([]string, error) {
	s3client, err := b.driver.CreateS3Client()
	if err != nil {
		return nil, err
	}

	bucketName, err := b.driver.GetBucketName(ctx, s3client)
	if bucketName == "" {
		return nil, err
	}

	return awsbyoc.ListPulumiStacks(ctx, s3client, bucketName)
}

func (b *ByocDo) CreateUploadURL(ctx context.Context, req *defangv1.UploadURLRequest) (*defangv1.UploadURLResponse, error) {
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

func (b *ByocDo) Delete(ctx context.Context, req *defangv1.DeleteRequest) (*defangv1.DeleteResponse, error) {
	return nil, client.ErrNotImplemented("not implemented for DigitalOcean")
}

func (b *ByocDo) Destroy(ctx context.Context, req *defangv1.DestroyRequest) (string, error) {
	return b.BootstrapCommand(ctx, client.BootstrapCommandRequest{Project: req.Project, Command: "down"})
}

func (b *ByocDo) DeleteConfig(ctx context.Context, secrets *defangv1.Secrets) error {
	//Create app, it fails, add secrets later
	// triggers an app update with the new config
	app, err := b.getAppByName(ctx, secrets.Project)

	toDelete := strings.Join(secrets.Names, ",")

	if err != nil {
		return err
	}

	deleteEnvVars(toDelete, &app.Spec.Envs)
	for _, service := range app.Spec.Services {
		deleteEnvVars(toDelete, &service.Envs)
	}

	_, _, err = b.client.Apps.Update(ctx, app.ID, &godo.AppUpdateRequest{Spec: app.Spec})

	return err
}

func (b *ByocDo) GetService(ctx context.Context, s *defangv1.GetRequest) (*defangv1.ServiceInfo, error) {
	//Dumps endpoint and tag. Reads the protobuff for that service. Combines with info from get app.
	//Only used in Tail
	app, err := b.getAppByName(ctx, s.Project)
	if err != nil {
		return nil, err
	}

	var serviceInfo *defangv1.ServiceInfo

	for _, service := range app.Spec.Services {
		if service.Name == s.Name {
			serviceInfo = processServiceInfo(service, s.Project)
		}
	}

	return serviceInfo, nil
}

func (b *ByocDo) getProjectInfo(ctx context.Context, services *[]*defangv1.ServiceInfo, projectName string) (*godo.App, error) {
	//Dumps endpoint and tag. Reads the protobuff for all services. Combines with info for g
	app, err := b.getAppByName(ctx, projectName)
	if err != nil {
		return nil, err
	}

	for _, service := range app.Spec.Services {
		serviceInfo := processServiceInfo(service, projectName)
		*services = append(*services, serviceInfo)
	}

	return app, nil
}

func (b *ByocDo) GetServices(ctx context.Context, req *defangv1.GetServicesRequest) (*defangv1.GetServicesResponse, error) {
	resp := defangv1.GetServicesResponse{}
	_, err := b.getProjectInfo(ctx, &resp.Services, req.Project)
	if err != nil {
		return nil, err
	}

	return &resp, err
}

func (b *ByocDo) ListConfig(ctx context.Context, req *defangv1.ListConfigsRequest) (*defangv1.Secrets, error) {
	app, err := b.getAppByName(ctx, req.Project)
	if err != nil {
		return nil, err
	}

	secrets := &defangv1.Secrets{}

	for _, envVar := range app.Spec.Envs {
		secrets.Names = append(secrets.Names, envVar.Key)
	}

	for _, service := range app.Spec.Services {
		for _, envVar := range service.Envs {
			secrets.Names = append(secrets.Names, envVar.Key)
		}
	}

	return secrets, nil
}

func (b *ByocDo) PutConfig(ctx context.Context, config *defangv1.PutConfigRequest) error {
	// redeploy app with updated config in pulumi "regular deployment"
	app, err := b.getAppByName(ctx, config.Project)
	if err != nil {
		return err
	}

	newSecret := &godo.AppVariableDefinition{
		Key:   config.Name,
		Value: config.Value,
		Type:  godo.AppVariableType_Secret,
	}

	app.Spec.Envs = append(app.Spec.Envs, newSecret)

	_, _, err = b.client.Apps.Update(ctx, app.ID, &godo.AppUpdateRequest{Spec: app.Spec})

	return err
}

func (b *ByocDo) QueryLogs(ctx context.Context, req *defangv1.TailRequest) (client.ServerStream[defangv1.TailResponse], error) {
	var appID, deploymentID string

	if req.Etag != "" && req.Etag == b.cdEtag {
		// Use the last known app and deployment ID from the last CD command
		appID = b.cdAppID
		deploymentID = b.cdDeploymentID
	}

	if deploymentID == "" || appID == "" {
		//Look up the CD app directly instead of relying on the etag
		term.Debug("Fetching app and deployment ID for app", appPlatform.CdName)
		cdApp, err := b.getAppByName(ctx, appPlatform.CdName)
		if err != nil {
			return nil, err
		}
		appID = cdApp.ID
		switch {
		case cdApp.PendingDeployment != nil:
			deploymentID = cdApp.PendingDeployment.ID
		case cdApp.InProgressDeployment != nil:
			deploymentID = cdApp.InProgressDeployment.ID
		case cdApp.ActiveDeployment != nil:
			deploymentID = cdApp.ActiveDeployment.ID
		}
	}

	if deploymentID == "" {
		return nil, errors.New("no deployments found")
	}

	term.Info("Waiting for CD command to finish gathering logs")
	for {
		deploymentInfo, _, err := b.client.Apps.GetDeployment(ctx, appID, deploymentID)
		if err != nil {
			return nil, err
		}

		logType := logs.LogType(req.LogType)

		term.Debugf("Deployment phase: %s", deploymentInfo.GetPhase())
		switch deploymentInfo.GetPhase() {
		case godo.DeploymentPhase_PendingBuild, godo.DeploymentPhase_PendingDeploy, godo.DeploymentPhase_Deploying:
			// Do nothing; check again in 10 seconds

		case godo.DeploymentPhase_Error, godo.DeploymentPhase_Canceled:
			if logType.Has(logs.LogTypeBuild) {
				// TODO: provide component name
				logs, _, err := b.client.Apps.GetLogs(ctx, appID, deploymentID, "", godo.AppLogTypeDeploy, true, 50)
				if err != nil {
					return nil, err
				}
				readHistoricalLogs(ctx, logs.HistoricURLs)
			}
			return nil, errors.New("deployment failed")

		case godo.DeploymentPhase_Active:
			if logType.Has(logs.LogTypeBuild) {
				logs, _, err := b.client.Apps.GetLogs(ctx, appID, deploymentID, "", godo.AppLogTypeDeploy, true, 50)
				if err != nil {
					return nil, err
				}
				readHistoricalLogs(ctx, logs.HistoricURLs)
			}

			appLiveURL, err := b.processServiceLogs(ctx, req.Project, logType)
			if err != nil {
				return nil, err
			}

			return newByocServerStream(ctx, appLiveURL, req.Etag)
		}

		// Sleep for 10 seconds so we dont spam the DO API
		if err := pkg.SleepWithContext(ctx, 10*time.Second); err != nil {
			return nil, err
		}
	}
}

func (b *ByocDo) TearDown(ctx context.Context) error {
	app, err := b.getAppByName(ctx, appPlatform.CdName)
	if err != nil {
		return err
	}

	_, err = b.client.Apps.Delete(ctx, app.ID)
	if err != nil {
		return err
	}

	_, err = b.client.Registry.Delete(ctx)
	if err != nil {
		return err
	}

	_, err = b.client.Projects.Delete(ctx, byoc.CdTaskPrefix)
	if err != nil {
		return err
	}

	return nil
}

func (b *ByocDo) AccountInfo(ctx context.Context) (*client.AccountInfo, error) {
	accessToken := os.Getenv("DIGITALOCEAN_TOKEN")
	if accessToken == "" {
		return nil, errors.New("DIGITALOCEAN_TOKEN must be set (https://docs.defang.io/docs/providers/digitalocean#getting-started)")
	}
	account, _, err := b.client.Account.Get(ctx)
	if err != nil {
		return nil, err
	}
	return &client.AccountInfo{
		AccountID: account.Email,
		Region:    b.driver.Region.String(),
		Provider:  client.ProviderDO,
	}, nil
}

func (b *ByocDo) Subscribe(ctx context.Context, req *defangv1.SubscribeRequest) (client.ServerStream[defangv1.SubscribeResponse], error) {
	if req.Etag != b.cdEtag || b.cdAppID == "" {
		return nil, errors.ErrUnsupported // TODO: fetch the deployment ID for the given etag
	}
	ctx, cancel := context.WithCancel(ctx) // canceled by subscribeStream.Close()
	return &subscribeStream{
		appID:        b.cdAppID,
		b:            b,
		deploymentID: b.cdDeploymentID,
		ctx:          ctx,
		cancel:       cancel,
		queue:        make(chan *defangv1.SubscribeResponse, 10),
	}, nil
}

type subscribeStream struct {
	appID        string
	b            *ByocDo
	ctx          context.Context
	cancel       context.CancelFunc
	deploymentID string
	err          error
	queue        chan *defangv1.SubscribeResponse
	msg          *defangv1.SubscribeResponse
}

func phaseToState(phase godo.DeploymentPhase) defangv1.ServiceState {
	switch phase {
	case godo.DeploymentPhase_Building:
		return defangv1.ServiceState_BUILD_RUNNING
	case godo.DeploymentPhase_Active:
		return defangv1.ServiceState_DEPLOYMENT_COMPLETED
	case godo.DeploymentPhase_Canceled:
		return defangv1.ServiceState_DEPLOYMENT_SCALED_IN
	case godo.DeploymentPhase_Error:
		return defangv1.ServiceState_DEPLOYMENT_FAILED
	case godo.DeploymentPhase_PendingBuild:
		return defangv1.ServiceState_BUILD_QUEUED
	case godo.DeploymentPhase_PendingDeploy:
		return defangv1.ServiceState_UPDATE_QUEUED
	case godo.DeploymentPhase_Deploying:
		return defangv1.ServiceState_DEPLOYMENT_PENDING
	default:
		return defangv1.ServiceState_NOT_SPECIFIED
	}
}

func (s *subscribeStream) Receive() bool {
	select {
	case <-s.ctx.Done():
		s.err = s.ctx.Err()
		s.msg = nil
		return false
	case r := <-s.queue:
		s.msg = r
		return true
	default:
	}
	deployment, _, err := s.b.client.Apps.GetDeployment(s.ctx, s.appID, s.deploymentID)
	if err != nil {
		s.msg = nil
		s.err = err
		return false
	}
	for _, service := range deployment.Spec.Services {
		s.queue <- &defangv1.SubscribeResponse{
			Name:   service.Name,
			Status: string(deployment.Phase),
			State:  phaseToState(deployment.Phase),
		}
	}

	select {
	case resp := <-s.queue:
		s.msg = resp
		return true
	case <-s.ctx.Done():
		s.err = s.ctx.Err()
		s.msg = nil
		return false
	}
}

func (s *subscribeStream) Msg() *defangv1.SubscribeResponse {
	return s.msg
}

func (s *subscribeStream) Err() error {
	return s.err
}

func (s *subscribeStream) Close() error {
	s.cancel()
	return nil
}

func (b *ByocDo) QueryForDebug(ctx context.Context, req *defangv1.DebugRequest) error {
	return client.ErrNotImplemented("AI debugging is not yet supported for DO BYOC")
}

func (b *ByocDo) PrepareDomainDelegation(ctx context.Context, req client.PrepareDomainDelegationRequest) (*client.PrepareDomainDelegationResponse, error) {
	return nil, nil // TODO: implement domain delegation for DO
}

func (b *ByocDo) runCdCommand(ctx context.Context, projectName, delegateDomain string, cmd ...string) (*godo.App, error) { // nolint:unparam
	env, err := b.environment(projectName, delegateDomain)
	if err != nil {
		return nil, err
	}
	if term.DoDebug() {
		// Convert the environment to a human-readable array of KEY=VALUE strings for debugging
		debugEnv := make([]string, len(env))
		for i, v := range env {
			debugEnv[i] = v.Key + "=" + v.Value
		}
		if err := byoc.DebugPulumi(ctx, debugEnv, cmd...); err != nil {
			return nil, err
		}
	}
	app, err := b.driver.Run(ctx, env, b.CDImage, append([]string{"node", "lib/index.js"}, cmd...)...)
	if err != nil {
		return nil, err
	}

	b.cdAppID = app.ID
	b.cdDeploymentID = app.PendingDeployment.ID
	return app, nil
}

func (b *ByocDo) environment(projectName, delegateDomain string) ([]*godo.AppVariableDefinition, error) {
	region := b.driver.Region // TODO: this should be the destination region, not the CD region; make customizable
	defangStateUrl := fmt.Sprintf(`s3://%s?endpoint=%s.digitaloceanspaces.com`, b.driver.BucketName, region)
	pulumiBackendKey, pulumiBackendValue, err := byoc.GetPulumiBackend(defangStateUrl)
	if err != nil {
		return nil, err
	}
	env := []*godo.AppVariableDefinition{
		{
			Key:   "DEFANG_PREFIX",
			Value: byoc.DefangPrefix,
		},
		{
			Key:   "DEFANG_DEBUG",
			Value: os.Getenv("DEFANG_DEBUG"),
		},
		{
			Key:   "DEFANG_JSON",
			Value: os.Getenv("DEFANG_JSON"),
		},
		{
			Key:   "DEFANG_ORG",
			Value: b.TenantName,
		},
		{
			Key:   "DOMAIN",
			Value: b.GetProjectDomain(projectName, delegateDomain),
		},
		{
			Key:   "PRIVATE_DOMAIN",
			Value: byoc.GetPrivateDomain(projectName),
		},
		{
			Key:   "PROJECT",
			Value: projectName,
		},
		{
			Key:   pulumiBackendKey,
			Value: pulumiBackendValue,
			Type:  godo.AppVariableType_Secret,
		},
		{
			Key:   "DEFANG_STATE_URL",
			Value: defangStateUrl,
		},
		{
			Key:   "PULUMI_CONFIG_PASSPHRASE",
			Value: byoc.PulumiConfigPassphrase,
			Type:  godo.AppVariableType_Secret,
		},
		{
			Key:   "STACK",
			Value: b.PulumiStack,
		},
		{
			Key:   "NODE_NO_WARNINGS",
			Value: "1",
		},
		{
			Key:   "NPM_CONFIG_UPDATE_NOTIFIER",
			Value: "false",
		},
		{
			Key:   "PULUMI_COPILOT",
			Value: "false",
		},
		{
			Key:   "PULUMI_SKIP_UPDATE_CHECK",
			Value: "true",
		},
		{
			Key:   "DIGITALOCEAN_TOKEN",
			Value: os.Getenv("DIGITALOCEAN_TOKEN"),
			Type:  godo.AppVariableType_Secret,
		},
		{
			Key:   "SPACES_ACCESS_KEY_ID",
			Value: os.Getenv("SPACES_ACCESS_KEY_ID"),
			Type:  godo.AppVariableType_Secret,
		},
		{
			Key:   "SPACES_SECRET_ACCESS_KEY",
			Value: os.Getenv("SPACES_SECRET_ACCESS_KEY"),
			Type:  godo.AppVariableType_Secret,
		},
		{
			Key:   "REGION",
			Value: region.String(),
		},
		{
			Key:   "DEFANG_BUILD_REPO",
			Value: b.buildRepo,
		},
		{
			Key:   "AWS_REGION", // Needed for CD S3 functions; FIXME: remove this
			Value: region.String(),
		},
		{
			Key:   "AWS_ACCESS_KEY_ID", // Needed for CD S3 functions; FIXME: remove this
			Value: os.Getenv("SPACES_ACCESS_KEY_ID"),
			Type:  godo.AppVariableType_Secret,
		},
		{
			Key:   "AWS_SECRET_ACCESS_KEY", // Needed for CD S3 functions; FIXME: remove this
			Value: os.Getenv("SPACES_SECRET_ACCESS_KEY"),
			Type:  godo.AppVariableType_Secret,
		},
	}
	if !term.StdoutCanColor() {
		env = append(env, &godo.AppVariableDefinition{Key: "NO_COLOR", Value: "1"})
	}
	return env, nil
}

func (b *ByocDo) setUp(ctx context.Context) error {
	if b.SetupDone {
		return nil
	}

	if err := b.driver.SetUpBucket(ctx); err != nil {
		return err
	}

	// Create the Container Registry here, because DO only allows a single one per account,
	// so we can't create it in the CD Pulumi process.
	registry, resp, err := b.client.Registry.Get(ctx)
	if err != nil {
		if resp.StatusCode != 404 {
			return err
		}
		term.Debug("Creating new registry")
		// Create registry if it doesn't exist
		registry, _, err = b.client.Registry.Create(ctx, &godo.RegistryCreateRequest{
			Name:                 pkg.RandomID(), // has to be globally unique
			SubscriptionTierSlug: "starter",      // max 1 repo; TODO: make this configurable
			Region:               b.driver.Region.String(),
		})
		if err != nil {
			return err
		}
	}

	b.buildRepo = registry.Name + "/kaniko-build" // TODO: use/add b.PulumiProject but only if !starter
	b.SetupDone = true

	return nil
}

func (b *ByocDo) getAppByName(ctx context.Context, name string) (*godo.App, error) {
	appName := name
	if !strings.Contains(name, "defang") {
		appName = fmt.Sprintf("%s-%s-%s-app", DEFANG, name, b.PulumiStack)
	}

	apps, _, err := b.client.Apps.List(ctx, &godo.ListOptions{})
	if err != nil {
		return nil, err
	}

	for _, app := range apps {
		if app.Spec.Name == appName {
			return app, nil
		}
	}

	return nil, fmt.Errorf("app not found: %s", appName)
}

func processServiceInfo(service *godo.AppServiceSpec, projectName string) *defangv1.ServiceInfo {
	serviceInfo := &defangv1.ServiceInfo{
		Project: projectName,
		Etag:    pkg.RandomID(), // TODO: get the real etag from spec somehow
		Service: &defangv1.Service{
			Name: service.Name,
		},
	}

	return serviceInfo
}

func (b *ByocDo) processServiceLogs(ctx context.Context, projectName string, logType logs.LogType) (string, error) {
	appLiveURL := ""

	buildAppName := fmt.Sprintf("defang-%s-%s-build", projectName, b.PulumiStack)
	mainAppName := fmt.Sprintf("defang-%s-%s-app", projectName, b.PulumiStack)

	// If we can get projects working, we can add the project to the list options
	currentApps, _, err := b.client.Apps.List(ctx, &godo.ListOptions{})
	if err != nil {
		return "", err
	}

	for _, app := range currentApps {
		if logType.Has(logs.LogTypeBuild) && app.Spec.Name == buildAppName {
			buildLogs, _, err := b.client.Apps.GetLogs(ctx, app.ID, "", "", godo.AppLogTypeDeploy, false, 50)
			if err != nil {
				return "", err
			}
			readHistoricalLogs(ctx, buildLogs.HistoricURLs)
		}

		if app.Spec.Name == mainAppName {
			deployments, _, err := b.client.Apps.ListDeployments(ctx, app.ID, &godo.ListOptions{})
			if err != nil {
				return "", err
			}

			if logType.Has(logs.LogTypeBuild) {
				mainDeployLogs, resp, err := b.client.Apps.GetLogs(ctx, app.ID, "", "", godo.AppLogTypeDeploy, true, 50)
				if err != nil {
					return "", err
				}
				if resp.StatusCode != 200 {
					// godo has no concept of returning the "last deployment", only "Active", "Pending", etc
					// Create our own last deployment and return deployment logs if the deployment failed in the last 2 minutes
					if deployments[0].Phase == godo.DeploymentPhase_Error && deployments[0].UpdatedAt.After(time.Now().Add(-2*time.Minute)) {
						failDeployLogs, _, err := b.client.Apps.GetLogs(ctx, app.ID, deployments[0].ID, "", godo.AppLogTypeDeploy, true, 50)
						if err != nil {
							return "", err
						}
						readHistoricalLogs(ctx, failDeployLogs.HistoricURLs)
					}
					// Assume no deploy happened, return without an error
					return "", nil
				}
				readHistoricalLogs(ctx, mainDeployLogs.HistoricURLs)
			}
			if logType.Has(logs.LogTypeRun) {
				mainRunLogs, resp, err := b.client.Apps.GetLogs(ctx, app.ID, "", "", godo.AppLogTypeRun, true, 50)
				if err != nil {
					return "", err
				}
				if resp.StatusCode != 200 {
					// Assume no deploy happened, return without an error
					return "", nil
				}
				readHistoricalLogs(ctx, mainRunLogs.HistoricURLs)
				appLiveURL = mainRunLogs.LiveURL
			}
		}
	}
	return appLiveURL, nil
}

func readHistoricalLogs(ctx context.Context, urls []string) {
	for _, u := range urls {
		resp, err := http.GetWithContext(ctx, u)
		if err != nil {
			continue
		}
		defer resp.Body.Close()
		// io.Copy(os.Stdout, resp.Body)
		tailResp := &defangv1.TailResponse{}
		body, _ := io.ReadAll(resp.Body)
		lines := strings.Split(string(body), "\n")
		for _, line := range lines {
			parts := strings.SplitN(line, " ", 3)
			if len(parts) == 1 {
				continue
			}
			ts, _ := time.Parse(time.RFC3339Nano, parts[1])
			tailResp.Entries = append(tailResp.Entries, &defangv1.LogEntry{
				Message:   parts[2],
				Timestamp: timestamppb.New(ts),
				Service:   parts[0],
			})
		}

		for _, msg := range tailResp.Entries {
			printlogs(msg)
		}
	}
}

func getServiceEnv(envVars []*godo.AppVariableDefinition) map[string]string { // nolint:unused
	env := make(map[string]string)
	for _, envVar := range envVars {
		env[envVar.Key] = envVar.Value
	}
	return env
}

func printlogs(msg *defangv1.LogEntry) {
	service := msg.Service
	etag := msg.Etag
	ts := msg.Timestamp.AsTime()
	tsString := ts.Local().Format(RFC3339Micro)
	tsColor := termenv.ANSIBrightBlack
	if term.HasDarkBackground() {
		tsColor = termenv.ANSIWhite
	}
	if msg.Stderr {
		tsColor = termenv.ANSIBrightRed
	}
	var prefixLen int
	trimmed := strings.TrimRight(msg.Message, "\t\r\n ")
	buf := term.NewMessageBuilder(term.StdoutCanColor())
	for i, line := range strings.Split(trimmed, "\n") {
		if i == 0 {
			prefixLen, _ = buf.Printc(tsColor, tsString, " ")
			l, _ := buf.Printc(termenv.ANSIYellow, etag, " ")
			prefixLen += l
			l, _ = buf.Printc(termenv.ANSIYellow, etag, " ")
			prefixLen += l
			l, _ = buf.Printc(termenv.ANSIGreen, service, " ")
			prefixLen += l
		} else {
			io.WriteString(buf, strings.Repeat(" ", prefixLen))
		}
		if term.StdoutCanColor() {
			if !strings.Contains(line, "\033[") {
				line = colorKeyRegex.ReplaceAllString(line, replaceString) // add some color
			}
		} else {
			line = term.StripAnsi(line)
		}
		io.WriteString(buf, line)
	}
	term.Println(buf.String())
}
