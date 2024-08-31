package do

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/digitalocean/godo"
	"io"
	"net/url"
	"os"
	"strings"
	"time"

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
		apps:   map[string]*godo.App{},
	}
	b.ByocBaseClient = byoc.NewByocBaseClient(ctx, grpcClient, tenantId, b)

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

	data, err := proto.Marshal(&defangv1.ListServicesResponse{
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

	term.Debug(fmt.Sprintf("PAYLOAD STRING: %s", payloadString))

	cdApp, err := b.runCdCommand(ctx, "up", payloadString)
	if err != nil {
		return nil, err
	}
	b.apps[etag] = cdApp

	return &defangv1.DeployResponse{
		Services: serviceInfos,
		Etag:     etag,
	}, nil
}

func (b *ByocDo) BootstrapCommand(ctx context.Context, command string) (string, error) {
	if err := b.setUp(ctx); err != nil {
		return "", err
	}

	cdApp, err := b.runCdCommand(ctx, command)
	if err != nil {
		return "", err
	}
	etag := pkg.RandomID()
	b.apps[etag] = cdApp

	return etag, nil
}

func (b *ByocDo) BootstrapList(ctx context.Context) ([]string, error) {
	// Use DO api to query which apps (or projects) exist based on defang constant
	return nil, client.ErrNotImplemented("not implemented for ByocDo")
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
	return errors.New("Digital Ocean does not currently support config.")
}

func (b *ByocDo) GetService(ctx context.Context, s *defangv1.ServiceID) (*defangv1.ServiceInfo, error) {
	//Dumps endpoint and tag. Reads the protobuff for that service. Combines with info from get app.
	return &defangv1.ServiceInfo{}, errors.ErrUnsupported
}

func (b *ByocDo) GetServices(ctx context.Context) (*defangv1.ListServicesResponse, error) {
	//Dumps endpoint and tag. Reads the protobuff for all services. Combines with info for g
	return &defangv1.ListServicesResponse{}, errors.ErrUnsupported
}

func (b *ByocDo) ListConfig(ctx context.Context) (*defangv1.Secrets, error) {
	//get app and return the environment
	return nil, errors.New("Digital Ocean does not currently support config.")
}

func (b *ByocDo) PutConfig(ctx context.Context, secret *defangv1.SecretValue) error {
	// redeploy app with updated config in pulumi "regular deployment"
	return errors.New("Digital Ocean does not currently support config.")
}

func (b *ByocDo) Restart(ctx context.Context, names ...string) (types.ETag, error) {
	return "", errors.ErrUnsupported
}

func (b *ByocDo) ServiceDNS(name string) string {
	return "localhost"
}

func (b *ByocDo) Follow(ctx context.Context, req *defangv1.TailRequest) (client.ServerStream[defangv1.TailResponse], error) {
	if err := b.setUp(ctx); err != nil {
		return nil, err
	}
	// This is an etag; look up the app ID
	cdApp := b.apps[req.Etag]
	if cdApp == nil {
		return nil, fmt.Errorf("unknown etag: %s", req.Etag)
	}

	term.Debug(fmt.Sprintf("FOLLOW APP ID: %s", cdApp.ID))
	var appLiveURL string
	term.Info("Waiting for Deploy to finish to gather logs")
	deploymentID := cdApp.PendingDeployment.ID
	for {

		deploymentInfo, _, err := b.driver.Client.Apps.GetDeployment(ctx, cdApp.ID, deploymentID)

		if err != nil {
			return nil, err
		}

		term.Debug(fmt.Sprintf("DEPLOYMENT ID: %s", deploymentID))

		if deploymentInfo.GetPhase() == godo.DeploymentPhase_Error {
			logs, _, err := b.driver.Client.Apps.GetLogs(ctx, cdApp.ID, "", "", godo.AppLogTypeDeploy, false, 150)
			if err != nil {
				return nil, err
			}
			readHistoricalLogs(ctx, logs.HistoricURLs)
			return nil, errors.New("problem deploying app")
		}

		if deploymentInfo.GetPhase() == godo.DeploymentPhase_Active {
			logs, _, err := b.driver.Client.Apps.GetLogs(ctx, cdApp.ID, "", "", godo.AppLogTypeDeploy, true, 50)

			if err != nil {
				return nil, err
			}

			readHistoricalLogs(ctx, logs.HistoricURLs)

			project, err := b.LoadProject(ctx)
			if err != nil {
				return nil, err
			}

			buildAppName := fmt.Sprintf("defang-%s-%s-build", project.Name, b.PulumiStack)
			mainAppName := fmt.Sprintf("defang-%s-%s-app", project.Name, b.PulumiStack)

			term.Debugf("BUILD APP NAME: %s", buildAppName)
			term.Debugf("MAIN APP NAME: %s", mainAppName)

			// If we can get projects working, we can add the project to the list options
			currentApps, _, err := b.driver.Client.Apps.List(ctx, &godo.ListOptions{})

			if err != nil {
				return nil, err
			}

			for _, app := range currentApps {
				term.Debugf("APP NAME: %s", app.Spec.Name)
				if app.Spec.Name == buildAppName {
					buildLogs, _, err := b.driver.Client.Apps.GetLogs(ctx, app.ID, "", "", godo.AppLogTypeDeploy, false, 50)
					if err != nil {
						return nil, err
					}
					readHistoricalLogs(ctx, buildLogs.HistoricURLs)
				}
				if app.Spec.Name == mainAppName {
					mainLogs, _, err := b.driver.Client.Apps.GetLogs(ctx, app.ID, "", "", godo.AppLogTypeRun, true, 50)
					if err != nil {
						return nil, err
					}
					readHistoricalLogs(ctx, mainLogs.HistoricURLs)
					appLiveURL = mainLogs.LiveURL
				}
			}

			break
		}
		//Sleep for 15 seconds so we dont spam the DO API
		pkg.SleepWithContext(ctx, (time.Second)*15)

	}
	term.Debug("LIVE URL:", appLiveURL)

	return newByocServerStream(ctx, appLiveURL, req.Etag)
}

func (b *ByocDo) TearDown(ctx context.Context) error {
	// kills the CD app as well, use DO api to remove CD
	return errors.ErrUnsupported
	//return b.Driver.TearDown(ctx)
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

func (b *ByocDo) runCdCommand(ctx context.Context, cmd ...string) (*godo.App, error) {
	env := b.environment()
	if term.DoDebug() {
		debugEnv := " -"
		for _, v := range env {
			debugEnv += " " + v.Key + "=" + v.Value
		}
		term.Debug(debugEnv, "npm run dev", strings.Join(cmd, " "))
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
			Value: b.PulumiProject,
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
			Key:   "AWS_REGION", // FIXME: why do we need this?
			Value: region.String(),
		},
		{
			Key:   "AWS_ACCESS_KEY_ID", // FIXME: why do we need this?
			Value: pkg.Getenv("SPACES_ACCESS_KEY_ID", os.Getenv("DO_SPACES_ID")),
			Type:  godo.AppVariableType_Secret,
		},
		{
			Key:   "AWS_SECRET_ACCESS_KEY", // FIXME: why do we need this?
			Value: pkg.Getenv("SPACES_SECRET_ACCESS_KEY", os.Getenv("DO_SPACES_KEY")),
			Type:  godo.AppVariableType_Secret,
		},
	}
}

func (b *ByocDo) update(ctx context.Context, service *defangv1.Service) (*defangv1.ServiceInfo, error) {

	si := &defangv1.ServiceInfo{
		Service: service,
		Project: b.PulumiProject,
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
	registry, _, err := b.driver.Client.Registry.Get(ctx)
	if err != nil {
		term.Debug("Registry.Get error:", err) // FIXME: check error
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

func readHistoricalLogs(ctx context.Context, urls []string) {
	for _, u := range urls {
		resp, err := http.GetWithContext(ctx, u)
		term.Debugf("GET LOGS STATUS CODE: %d", resp.StatusCode)
		if err != nil {
			continue
		}
		defer resp.Body.Close()
		io.Copy(os.Stdout, resp.Body)
	}
}
