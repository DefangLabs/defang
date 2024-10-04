package do

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/digitalocean/godo"
	"github.com/muesli/termenv"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc"

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

	apps      map[string]*godo.App
	buildRepo string
	driver    *appPlatform.DoApp
}

func NewByoc(ctx context.Context, grpcClient client.GrpcClient, tenantId types.TenantID) *ByocDo {
	doRegion := do.Region(os.Getenv("REGION"))
	if doRegion == "" {
		doRegion = region.SFO3
	}

	b := &ByocDo{
		driver: appPlatform.New(byoc.CdTaskPrefix, doRegion),
	}
	b.ByocBaseClient = byoc.NewByocBaseClient(ctx, grpcClient, tenantId, b)
	b.ProjectName, _ = b.LoadProjectName(ctx)
	return b
}

func (b *ByocDo) Deploy(ctx context.Context, req *defangv1.DeployRequest) (*defangv1.DeployResponse, error) {
	if err := b.setUp(ctx); err != nil {
		return nil, err
	}

	etag := pkg.RandomID()

	serviceInfos := []*defangv1.ServiceInfo{}
	//var warnings Warnings

	for _, service := range req.Services {
		serviceInfo, err := b.update(ctx, service)
		if err != nil {
			return nil, err
		}
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
		Services: serviceInfos,
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

	_, err = b.runCdCommand(ctx, "up", payloadString)
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

	projects, _, err := b.driver.Client.Projects.List(ctx, &godo.ListOptions{})
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
	// Unsupported in DO
	return &defangv1.DeleteResponse{}, errors.ErrUnsupported
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

	_, _, err = b.driver.Client.Apps.Update(ctx, app.ID, &godo.AppUpdateRequest{Spec: app.Spec})

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

func (b *ByocDo) GetServices(ctx context.Context) (*defangv1.ListServicesResponse, error) {
	//Dumps endpoint and tag. Reads the protobuff for all services. Combines with info for g
	app, err := b.getAppByName(ctx, b.ProjectName)
	if err != nil {
		return nil, err
	}
	services := &defangv1.ListServicesResponse{}

	for _, service := range app.Spec.Services {
		serviceInfo := b.processServiceInfo(service)
		services.Services = append(services.Services, serviceInfo)
	}

	return services, nil
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

	_, _, err = b.driver.Client.Apps.Update(ctx, app.ID, &godo.AppUpdateRequest{Spec: app.Spec})

	return err
}

func (b *ByocDo) Restart(ctx context.Context, names ...string) (types.ETag, error) {
	app, err := b.getAppByName(ctx, b.ProjectName)
	if err != nil {
		return "", err
	}

	_, _, err = b.driver.Client.Apps.Update(ctx, app.ID, &godo.AppUpdateRequest{Spec: app.Spec})

	return pkg.RandomID(), err
}

func (b *ByocDo) ServiceDNS(name string) string {
	return "localhost"
}

func (b *ByocDo) Follow(ctx context.Context, req *defangv1.TailRequest) (client.ServerStream[defangv1.TailResponse], error) {
	if err := b.setUp(ctx); err != nil {
		return nil, err
	}

	//Look up the CD app directly instead of relying on the etag
	cdApp, err := b.getAppByName(ctx, appPlatform.CDName)
	if err != nil {
		return nil, err
	}

	var appLiveURL string
	term.Info("Waiting for command to finish to gather logs")

	deploymentID := ""

	if cdApp.PendingDeployment != nil {
		deploymentID = cdApp.PendingDeployment.GetID()
	}

	if deploymentID == "" && cdApp.ActiveDeployment != nil {
		deploymentID = cdApp.ActiveDeployment.GetID()
	}

	if deploymentID == "" {
		return nil, errors.New("No deployments found")
	}

	for {

		deploymentInfo, _, err := b.driver.Client.Apps.GetDeployment(ctx, cdApp.ID, deploymentID)

		if err != nil {
			return nil, err
		}

		if deploymentInfo.GetPhase() == godo.DeploymentPhase_Error {
			logs, _, err := b.driver.Client.Apps.GetLogs(ctx, cdApp.ID, deploymentID, "", godo.AppLogTypeDeploy, false, 150)
			if err != nil {
				return nil, err
			}

			// Return build and app logs if there are any
			_, err = b.processServiceLogs(ctx)
			if err != nil {
				return nil, err
			}

			readHistoricalLogs(ctx, logs.HistoricURLs)
			return nil, errors.New("problem deploying app")
		}

		if deploymentInfo.GetPhase() == godo.DeploymentPhase_Active {

			logs, _, err := b.driver.Client.Apps.GetLogs(ctx, cdApp.ID, deploymentID, "", godo.AppLogTypeDeploy, true, 50)
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
		pkg.SleepWithContext(ctx, (time.Second)*15)

	}

	return newByocServerStream(ctx, appLiveURL, req.Etag)
}

func (b *ByocDo) TearDown(ctx context.Context) error {
	_, err := b.BootstrapCommand(ctx, "down")
	if err != nil {
		return err
	}

	app, err := b.getAppByName(ctx, appPlatform.CDName)
	if err != nil {
		return err
	}

	_, err = b.driver.Client.Registry.Delete(ctx)
	if err != nil {
		return err
	}

	_, err = b.driver.Client.Apps.Delete(ctx, app.ID)
	if err != nil {
		return err
	}

	_, err = b.driver.Client.Projects.Delete(ctx, byoc.CdTaskPrefix)
	if err != nil {
		return err
	}

	return nil
}

func (b *ByocDo) WhoAmI(ctx context.Context) (*defangv1.WhoAmIResponse, error) {
	if _, err := b.GrpcClient.WhoAmI(ctx); err != nil {
		return nil, err
	}

	return &defangv1.WhoAmIResponse{
		Tenant:  b.TenantID,
		Region:  b.driver.Region.String(),
		Account: "Digital Ocean",
	}, nil
}

func (b *ByocDo) GetVersion(context.Context) (*defangv1.Version, error) {
	cdVersion := byoc.CdImage[strings.LastIndex(byoc.CdImage, ":")+1:]
	return &defangv1.Version{Fabric: cdVersion}, nil
}

func (b *ByocDo) Subscribe(context.Context, *defangv1.SubscribeRequest) (client.ServerStream[defangv1.SubscribeResponse], error) {
	//optional
	return nil, errors.ErrUnsupported
}

func (b *ByocDo) runLocalPulumiCommand(ctx context.Context, dir string, cmd ...string) error {
	return errors.ErrUnsupported // TODO: implement for Windows
	// driver := local.New()
	// if err := driver.SetUp(ctx, []types.Container{{
	// 	EntryPoint: []string{"npm", "run", "dev"},
	// 	WorkDir:    dir,
	// }}); err != nil {
	// 	return err
	// }
	// localEnv := map[string]string{
	// 	"PATH": os.Getenv("PATH"),
	// }
	// for _, v := range b.environment() {
	// 	localEnv[v.Key] = v.Value
	// }
	// pid, err := driver.Run(ctx, localEnv, cmd...)
	// if err != nil {
	// 	return err
	// }
	// return driver.Tail(ctx, pid)
}

func (b *ByocDo) runCdCommand(ctx context.Context, cmd ...string) (*godo.App, error) {
	env := b.environment()
	if term.DoDebug() {
		debugEnv := " -"
		for _, v := range env {
			debugEnv += " " + v.Key + "=" + v.Value
		}
		term.Debug(debugEnv, "npm run dev", strings.Join(cmd, " "))
	}
	if dir := os.Getenv("DEFANG_PULUMI_DIR"); dir != "" {
		return nil, b.runLocalPulumiCommand(ctx, dir, cmd...)
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
			Value: pkg.Getenv("DIGITALOCEAN_TOKEN", os.Getenv("DO_PAT")),
		},
		{
			Key:   "SPACES_ACCESS_KEY_ID",
			Value: pkg.Getenv("SPACES_ACCESS_KEY_ID", os.Getenv("DO_SPACES_ID")),
			Type:  godo.AppVariableType_Secret,
		},
		{
			Key:   "SPACES_SECRET_ACCESS_KEY",
			Value: pkg.Getenv("SPACES_SECRET_ACCESS_KEY", os.Getenv("DO_SPACES_KEY")),
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
			Value: pkg.Getenv("SPACES_ACCESS_KEY_ID", os.Getenv("DO_SPACES_ID")),
			Type:  godo.AppVariableType_Secret,
		},
		{
			Key:   "AWS_SECRET_ACCESS_KEY", // Needed for CD S3 functions
			Value: pkg.Getenv("SPACES_SECRET_ACCESS_KEY", os.Getenv("DO_SPACES_KEY")),
			Type:  godo.AppVariableType_Secret,
		},
	}
}

func (b *ByocDo) update(ctx context.Context, service *defangv1.Service) (*defangv1.ServiceInfo, error) {

	si := &defangv1.ServiceInfo{
		Service: service,
		Project: b.ProjectName,
		Etag:    pkg.RandomID(),
	}

	//hasIngress := false
	//fqn := service.Name
	return si, nil
}

func (b *ByocDo) setUp(ctx context.Context) error {
	if b.SetupDone {
		return nil
	}

	if err := b.driver.SetUp(ctx); err != nil {
		return err
	}

	// Create the Container Registry here, because DO only allows a single one per account,
	// so we can't create it in the CD Pulumi process.
	registry, resp, err := b.driver.Client.Registry.Get(ctx)
	if err != nil {
		if resp.StatusCode != 404 {
			return err
		}
		term.Debug("Creating new registry")
		// Create registry if it doesn't exist
		registry, _, err = b.driver.Client.Registry.Create(ctx, &godo.RegistryCreateRequest{
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

	apps, _, err := b.driver.Client.Apps.List(ctx, &godo.ListOptions{})
	if err != nil {
		return nil, err
	}

	for _, app := range apps {
		if app.Spec.Name == appName {
			return app, nil
		}
	}

	return nil, errors.New(fmt.Sprintf("app not found: %s", appName))
}

func (b *ByocDo) processServiceInfo(service *godo.AppServiceSpec) *defangv1.ServiceInfo {

	serviceInfo := &defangv1.ServiceInfo{
		Project: b.ProjectName,
		Etag:    pkg.RandomID(),
		Service: &defangv1.Service{
			Name:        service.Name,
			Image:       service.Image.Digest,
			Environment: getServiceEnv(service.Envs),
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
	currentApps, _, err := b.driver.Client.Apps.List(ctx, &godo.ListOptions{})

	if err != nil {
		return "", err
	}

	for _, app := range currentApps {
		if app.Spec.Name == buildAppName {
			buildLogs, _, err := b.driver.Client.Apps.GetLogs(ctx, app.ID, "", "", godo.AppLogTypeDeploy, false, 50)
			if err != nil {
				return "", err
			}
			readHistoricalLogs(ctx, buildLogs.HistoricURLs)
		}
		if app.Spec.Name == mainAppName {
			mainLogs, _, err := b.driver.Client.Apps.GetLogs(ctx, app.ID, "", "", godo.AppLogTypeRun, true, 50)
			if err != nil {
				return "", err
			}
			readHistoricalLogs(ctx, mainLogs.HistoricURLs)
			appLiveURL = mainLogs.LiveURL
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
			Printlogs(tailResp, msg)
		}
	}

}

func getServiceEnv(envVars []*godo.AppVariableDefinition) map[string]string {
	env := make(map[string]string)
	for _, envVar := range envVars {
		env[envVar.Key] = envVar.Value
	}
	return env
}

func Printlogs(resp *defangv1.TailResponse, msg *defangv1.LogEntry) {
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
