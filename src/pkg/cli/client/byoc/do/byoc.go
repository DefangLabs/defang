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
	"github.com/DefangLabs/defang/src/pkg/cli/compose"

	"github.com/DefangLabs/defang/src/pkg/clouds/aws"
	"github.com/DefangLabs/defang/src/pkg/clouds/do"
	"github.com/DefangLabs/defang/src/pkg/clouds/do/appPlatform"
	"github.com/DefangLabs/defang/src/pkg/clouds/do/region"
	"github.com/DefangLabs/defang/src/pkg/http"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	composeTypes "github.com/compose-spec/compose-go/v2/types"
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

	buildRepo  string
	cdImageTag string
	client     *godo.Client
	driver     *appPlatform.DoApp
}

func NewByocProvider(ctx context.Context, grpcClient client.GrpcClient, tenantId types.TenantID) (*ByocDo, error) {
	doRegion := do.Region(os.Getenv("REGION"))
	if doRegion == "" {
		doRegion = region.SFO3 // TODO: change default
	}

	client, err := appPlatform.NewClient(ctx)
	if err != nil {
		return nil, err
	}

	b := &ByocDo{
		client: client,
		driver: appPlatform.New(doRegion),
	}
	b.ByocBaseClient = byoc.NewByocBaseClient(ctx, grpcClient, tenantId, b)
	b.ProjectName, _ = b.LoadProjectName(ctx)
	return b, nil
}

func (b *ByocDo) getCdImageTag(ctx context.Context) (string, error) {
	if b.cdImageTag != "" {
		return b.cdImageTag, nil
	}

	projUpdate, err := b.getProjectUpdate(ctx)
	if err != nil {
		return "", err
	}

	// older deployments may not have the cd_version field set,
	// these would have been deployed with public-beta
	if projUpdate != nil && projUpdate.CdVersion == "" {
		projUpdate.CdVersion = byoc.CdDefaultImageTag
	}

	// send project update with the current deploy's cd version,
	// most current version if new deployment
	imagePath := byoc.GetCdImage(appPlatform.CdImageBase, byoc.CdLatestImageTag)
	deploymentCdImageTag := byoc.ExtractImageTag(imagePath)
	if projUpdate != nil && len(projUpdate.Services) > 0 {
		deploymentCdImageTag = projUpdate.CdVersion
	}

	// possible values are [public-beta, 1, 2, ...]
	return deploymentCdImageTag, nil
}

func (b *ByocDo) getProjectUpdate(ctx context.Context) (*defangv1.ProjectUpdate, error) {
	client, err := b.driver.CreateS3Client()
	if err != nil {
		return nil, err
	}

	bucketName, err := b.driver.GetBucketName(ctx, client)
	if err != nil {
		return nil, err
	}

	if bucketName == "" {
		// bucket is not created yet; return empty update in that case
		return nil, nil // no services yet
	}

	path := fmt.Sprintf("projects/%s/%s/project.pb", b.ProjectName, b.PulumiStack)
	getObjectOutput, err := client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: &bucketName,
		Key:    &path,
	})

	if err != nil {
		if aws.IsS3NoSuchKeyError(err) {
			term.Debug("s3.GetObject:", err)
			return nil, nil // no services yet
		}
		return nil, byoc.AnnotateAwsError(err)
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
	if err := b.setUp(ctx); err != nil {
		return nil, err
	}

	// If multiple Compose files were provided, req.Compose is the merged representation of all the files
	project, err := compose.LoadFromContent(ctx, req.Compose)
	if err != nil {
		return nil, err
	}

	etag := pkg.RandomID()

	serviceInfos := []*defangv1.ServiceInfo{}

	for _, service := range project.Services {
		serviceInfo := b.update(service)
		serviceInfo.Etag = etag
		serviceInfos = append(serviceInfos, serviceInfo)
	}

	// Ensure all service endpoints are unique
	endpoints := make(map[string]bool)
	for _, serviceInfo := range serviceInfos {
		for _, endpoint := range serviceInfo.Endpoints {
			if endpoints[endpoint] {
				return nil, fmt.Errorf("duplicate endpoint: %s", endpoint) // CodeInvalidArgument
			}
			endpoints[endpoint] = true
		}
	}

	data, err := proto.Marshal(&defangv1.ProjectUpdate{
		CdVersion: b.cdImageTag,
		Compose:   req.Compose,
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

	_, err = b.runCdCommand(ctx, cmd, payloadString)
	if err != nil {
		return nil, err
	}

	return &defangv1.DeployResponse{
		Services: serviceInfos,
		Etag:     etag,
	}, nil
}

func (b *ByocDo) BootstrapCommand(ctx context.Context, command string) (string, error) {
	if err := b.setUp(ctx); err != nil {
		return "", err
	}

	_, err := b.runCdCommand(ctx, command)
	if err != nil {
		return "", err
	}
	etag := pkg.RandomID()

	return etag, nil
}

func (b *ByocDo) BootstrapList(ctx context.Context) ([]string, error) {
	// Use DO api to query which apps (or projects) exist based on defang constant

	var projectList []string

	projects, _, err := b.client.Projects.List(ctx, &godo.ListOptions{})
	if err != nil {
		return nil, err
	}

	for _, project := range projects {
		if strings.Contains(project.Name, "Defang") {
			projectList = append(projectList, project.Name)
		}
	}

	return projectList, nil
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

func (b *ByocDo) Destroy(ctx context.Context) (string, error) {
	return b.BootstrapCommand(ctx, "down")
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

func (b *ByocDo) GetService(ctx context.Context, s *defangv1.ServiceID) (*defangv1.ServiceInfo, error) {
	//Dumps endpoint and tag. Reads the protobuff for that service. Combines with info from get app.
	//Only used in Tail
	app, err := b.getAppByName(ctx, b.ProjectName)
	if err != nil {
		return nil, err
	}

	var serviceInfo *defangv1.ServiceInfo

	for _, service := range app.Spec.Services {
		if service.Name == s.Name {
			serviceInfo = b.processServiceInfo(service)
		}
	}

	return serviceInfo, nil
}

func (b *ByocDo) getProjectInfo(ctx context.Context, services *[]*defangv1.ServiceInfo) (*godo.App, error) {
	//Dumps endpoint and tag. Reads the protobuff for all services. Combines with info for g
	app, err := b.getAppByName(ctx, b.ProjectName)
	if err != nil {
		return nil, err
	}

	for _, service := range app.Spec.Services {
		serviceInfo := b.processServiceInfo(service)
		*services = append(*services, serviceInfo)
	}

	return app, nil
}

func (b *ByocDo) GetServices(ctx context.Context) (*defangv1.ListServicesResponse, error) {
	resp := defangv1.ListServicesResponse{}
	_, err := b.getProjectInfo(ctx, &resp.Services)
	if err != nil {
		return nil, err
	}

	return &resp, err
}

func (b *ByocDo) ListConfig(ctx context.Context) (*defangv1.Secrets, error) {
	app, err := b.getAppByName(ctx, b.ProjectName)
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

func (b *ByocDo) ServiceDNS(name string) string {
	return name // FIXME: what name should we use?
}

func (b *ByocDo) Follow(ctx context.Context, req *defangv1.TailRequest) (client.ServerStream[defangv1.TailResponse], error) {
	if err := b.setUp(ctx); err != nil {
		return nil, err
	}

	//Look up the CD app directly instead of relying on the etag
	cdApp, err := b.getAppByName(ctx, appPlatform.CdName)
	if err != nil {
		return nil, err
	}

	var appLiveURL, deploymentID string

	if cdApp.PendingDeployment != nil {
		deploymentID = cdApp.PendingDeployment.GetID()
	}

	if deploymentID == "" && cdApp.ActiveDeployment != nil {
		deploymentID = cdApp.ActiveDeployment.GetID()
	}

	if deploymentID == "" {
		return nil, errors.New("no deployments found")
	}

	term.Info("Waiting for CD command to finish gathering logs")
	for {
		deploymentInfo, _, err := b.client.Apps.GetDeployment(ctx, cdApp.ID, deploymentID)
		if err != nil {
			return nil, err
		}

		if deploymentInfo.GetPhase() == godo.DeploymentPhase_Active {
			logs, _, err := b.client.Apps.GetLogs(ctx, cdApp.ID, deploymentID, "", godo.AppLogTypeDeploy, true, 50)
			if err != nil {
				return nil, err
			}

			appLiveURL, err = b.processServiceLogs(ctx)
			if err != nil {
				return nil, err
			}

			readHistoricalLogs(ctx, logs.HistoricURLs)
			break
		}

		//Sleep for 15 seconds so we dont spam the DO API
		if err := pkg.SleepWithContext(ctx, (time.Second)*15); err != nil {
			return nil, err
		}
	}

	return newByocServerStream(ctx, appLiveURL, req.Etag)
}

func (b *ByocDo) TearDown(ctx context.Context) error {
	_, err := b.BootstrapCommand(ctx, "down")
	if err != nil {
		return err
	}

	app, err := b.getAppByName(ctx, appPlatform.CdName)
	if err != nil {
		return err
	}

	_, err = b.client.Registry.Delete(ctx)
	if err != nil {
		return err
	}

	_, err = b.client.Apps.Delete(ctx, app.ID)
	if err != nil {
		return err
	}

	_, err = b.client.Projects.Delete(ctx, byoc.CdTaskPrefix)
	if err != nil {
		return err
	}

	return nil
}

func (b *ByocDo) AccountInfo(ctx context.Context) (client.AccountInfo, error) {
	return DoAccountInfo{region: b.driver.Region.String()}, nil
}

type DoAccountInfo struct {
	region string
	// accountID string TODO: Find out the best field to be used as account id from https://docs.digitalocean.com/reference/api/api-reference/#tag/Account
}

func (i DoAccountInfo) AccountID() string {
	return "DigitalOcean"
}

func (i DoAccountInfo) Region() string {
	return i.region
}

func (i DoAccountInfo) Details() string {
	return ""
}

func (b *ByocDo) Subscribe(context.Context, *defangv1.SubscribeRequest) (client.ServerStream[defangv1.SubscribeResponse], error) {
	//optional
	return nil, errors.New("please check the Activity tab in the DigitalOcean App Platform console")
}

func (b *ByocDo) runCdCommand(ctx context.Context, cmd ...string) (*godo.App, error) { // nolint:unparam
	env := b.environment()
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
	app, err := b.driver.Run(ctx, env, append([]string{"node", "lib/index.js"}, cmd...)...)
	return app, err
}

func (b *ByocDo) environment() []*godo.AppVariableDefinition {
	region := b.driver.Region // TODO: this should be the destination region, not the CD region; make customizable
	return []*godo.AppVariableDefinition{
		{
			Key:   "DEFANG_PREFIX",
			Value: byoc.DefangPrefix,
		},
		{
			Key:   "DEFANG_DEBUG",
			Value: pkg.Getenv("DEFANG_DEBUG", "false"),
		},
		{
			Key:   "DEFANG_ORG",
			Value: b.TenantID,
		},
		{
			Key:   "DOMAIN",
			Value: b.ProjectDomain,
		},
		{
			Key:   "PRIVATE_DOMAIN",
			Value: b.PrivateDomain,
		},
		{
			Key:   "PROJECT",
			Value: b.ProjectName,
		},
		{
			Key:   "PULUMI_BACKEND_URL",
			Value: fmt.Sprintf(`s3://%s?endpoint=%s.digitaloceanspaces.com`, b.driver.BucketName, region),
		},
		{
			Key:   "PULUMI_CONFIG_PASSPHRASE",
			Value: pkg.Getenv("PULUMI_CONFIG_PASSPHRASE", "asdf"),
		},
		{
			Key:   "STACK",
			Value: b.PulumiStack,
		},
		{
			Key:   "NPM_CONFIG_UPDATE_NOTIFIER",
			Value: "false",
		},
		{
			Key:   "PULUMI_SKIP_UPDATE_CHECK",
			Value: "true",
		},
		{
			Key:   "DIGITALOCEAN_TOKEN",
			Value: os.Getenv("DIGITALOCEAN_TOKEN"),
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
			Key:   "AWS_REGION", // Needed for CD S3 functions
			Value: region.String(),
		},
		{
			Key:   "AWS_ACCESS_KEY_ID", // Needed for CD S3 functions
			Value: os.Getenv("SPACES_ACCESS_KEY_ID"),
			Type:  godo.AppVariableType_Secret,
		},
		{
			Key:   "AWS_SECRET_ACCESS_KEY", // Needed for CD S3 functions
			Value: os.Getenv("SPACES_SECRET_ACCESS_KEY"),
			Type:  godo.AppVariableType_Secret,
		},
	}
}

func (b *ByocDo) update(service composeTypes.ServiceConfig) *defangv1.ServiceInfo {
	si := &defangv1.ServiceInfo{
		Etag:    pkg.RandomID(),
		Project: b.ProjectName,
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

func (b *ByocDo) setUp(ctx context.Context) error {
	projectCdImageTag, err := b.getCdImageTag(ctx)
	if err != nil {
		return err
	}

	if b.SetupDone && b.cdImageTag == projectCdImageTag {
		return nil
	}

	if err := b.driver.SetUp(ctx); err != nil {
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

	b.cdImageTag = projectCdImageTag
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

func (b *ByocDo) processServiceInfo(service *godo.AppServiceSpec) *defangv1.ServiceInfo {
	serviceInfo := &defangv1.ServiceInfo{
		Project: b.ProjectName,
		Etag:    pkg.RandomID(),
		Service: &defangv1.Service{
			Name: service.Name,
			// Image:       service.Image.Digest,
			// Environment: getServiceEnv(service.Envs),
		},
	}

	return serviceInfo
}

func (b *ByocDo) processServiceLogs(ctx context.Context) (string, error) {
	project, err := b.LoadProject(ctx)
	appLiveURL := ""

	if err != nil {
		return "", err
	}

	buildAppName := fmt.Sprintf("defang-%s-%s-build", project.Name, b.PulumiStack)
	mainAppName := fmt.Sprintf("defang-%s-%s-app", project.Name, b.PulumiStack)

	// If we can get projects working, we can add the project to the list options
	currentApps, _, err := b.client.Apps.List(ctx, &godo.ListOptions{})
	if err != nil {
		return "", err
	}

	for _, app := range currentApps {
		if app.Spec.Name == buildAppName {
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

			mainDeployLogs, resp, err := b.client.Apps.GetLogs(ctx, app.ID, "", "", godo.AppLogTypeDeploy, true, 50)
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
			if err != nil {
				return "", err
			}

			readHistoricalLogs(ctx, mainDeployLogs.HistoricURLs)

			mainRunLogs, resp, err := b.client.Apps.GetLogs(ctx, app.ID, "", "", godo.AppLogTypeRun, true, 50)
			if resp.StatusCode != 200 {
				// Assume no deploy happened, return without an error
				return "", nil
			}
			if err != nil {
				return "", err
			}
			readHistoricalLogs(ctx, mainRunLogs.HistoricURLs)
			appLiveURL = mainRunLogs.LiveURL
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
