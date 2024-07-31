package do

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/digitalocean/godo"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc"

	"github.com/DefangLabs/defang/src/pkg/clouds/do"
	"github.com/DefangLabs/defang/src/pkg/clouds/do/appPlatform"
	"github.com/DefangLabs/defang/src/pkg/http"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"google.golang.org/protobuf/proto"
)

type ByocDo struct {
	*byoc.ByocBaseClient

	appIds map[string]string
	driver *appPlatform.DoApp
}

func NewByoc(ctx context.Context, grpcClient client.GrpcClient, tenantId types.TenantID) *ByocDo {
	regionString := os.Getenv("REGION")

	if regionString == "" {
		regionString = "sfo3"
	}

	b := &ByocDo{
		driver: appPlatform.New(byoc.CdTaskPrefix, do.Region(regionString)),
		appIds: map[string]string{},
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

	appID, err := b.runCdCommand(ctx, "up", payloadString)
	if err != nil {
		return nil, err
	}
	b.appIds[etag] = *appID

	return &defangv1.DeployResponse{
		Services: serviceInfos,
		Etag:     etag,
	}, nil
}

func (b *ByocDo) BootstrapCommand(ctx context.Context, command string) (string, error) {
	if err := b.setUp(ctx); err != nil {
		return "", err
	}

	foo, err := b.runCdCommand(ctx, "up", command)
	if err != nil {
		return "", err
	}

	return *foo, nil
}

func (b *ByocDo) BootstrapList(ctx context.Context) ([]string, error) {
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
	return &defangv1.DeleteResponse{}, errors.ErrUnsupported
}

func (b *ByocDo) Destroy(ctx context.Context) (string, error) {
	return b.BootstrapCommand(ctx, "down")
}

func (b *ByocDo) DeleteConfig(ctx context.Context, secrets *defangv1.Secrets) error {
	return errors.ErrUnsupported
}

func (b *ByocDo) GetService(ctx context.Context, s *defangv1.ServiceID) (*defangv1.ServiceInfo, error) {
	return &defangv1.ServiceInfo{}, errors.ErrUnsupported
}

func (b *ByocDo) GetServices(ctx context.Context) (*defangv1.ListServicesResponse, error) {
	return &defangv1.ListServicesResponse{}, errors.ErrUnsupported
}

func (b *ByocDo) ListConfig(ctx context.Context) (*defangv1.Secrets, error) {
	return &defangv1.Secrets{}, errors.ErrUnsupported
}

func (b *ByocDo) PutConfig(ctx context.Context, secret *defangv1.SecretValue) error {
	return nil
}

func (b *ByocDo) Restart(ctx context.Context, names ...string) (types.ETag, error) {
	return "", errors.ErrUnsupported
}

func (b *ByocDo) ServiceDNS(name string) string {
	return ""
}

func (b *ByocDo) Follow(ctx context.Context, req *defangv1.TailRequest) (client.ServerStream[defangv1.TailResponse], error) {
	if err := b.setUp(ctx); err != nil {
		return nil, err
	}

	appId := req.Etag
	if pkg.IsValidRandomID(appId) {
		// This is an etag; look up the app ID
		appId = b.appIds[appId]
	}

	// ctx, cancel := context.WithCancel(ctx)
	// defer cancel()

	client := b.driver.Client

	logs, _, err := client.Apps.GetLogs(ctx, appId, "", "", godo.AppLogTypeRun, true, 5)
	if err != nil {
		// assume not found; try again after a while
		pkg.SleepWithContext(ctx, 2*time.Second)
		logs, _, err = client.Apps.GetLogs(ctx, appId, "", "", godo.AppLogTypeRun, true, 5)
		if err != nil {
			return nil, err
		}
	}
	term.Debug("LIVE URL")
	term.Debug(logs.LiveURL)
	//newByocServerStream(ctx, logs, nil)

	return newByocServerStream(ctx, logs.LiveURL, []string{})
}

func (b *ByocDo) TearDown(ctx context.Context) error {
	return errors.ErrUnsupported
	//return b.Driver.TearDown(ctx)
}

func (b *ByocDo) WhoAmI(ctx context.Context) (*defangv1.WhoAmIResponse, error) {

	return &defangv1.WhoAmIResponse{}, errors.ErrUnsupported
}

func (b *ByocDo) GetVersion(context.Context) (*defangv1.Version, error) {
	cdVersion := byoc.CdImage[strings.LastIndex(byoc.CdImage, ":")+1:]
	return &defangv1.Version{Fabric: cdVersion}, nil
}

func (b *ByocDo) Get(ctx context.Context, s *defangv1.ServiceID) (*defangv1.ServiceInfo, error) {
	return &defangv1.ServiceInfo{}, errors.ErrUnsupported
}

func (b *ByocDo) Subscribe(context.Context, *defangv1.SubscribeRequest) (client.ServerStream[defangv1.SubscribeResponse], error) {
	return nil, errors.ErrUnsupported
}

func (b *ByocDo) runCdCommand(ctx context.Context, cmd ...string) (types.TaskID, error) {
	env := b.environment()
	if term.DoDebug() {
		debugEnv := " -"
		for _, v := range env {
			debugEnv += " " + v.Key + "=" + v.Value
		}
		term.Debug(debugEnv, "npm run dev", strings.Join(cmd, " "))
	}
	return b.driver.Run(ctx, env, append([]string{"node", "lib/index.js"}, cmd...)...)
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
			Key:   "DEFANG_CLOUD",
			Value: "do",
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
			Value: os.Getenv("DO_PAT"),
		},
		{
			Key:   "SPACES_ACCESS_KEY_ID",
			Value: pkg.Getenv("SPACES_ACCESS_KEY_ID", ""),
			Type:  godo.AppVariableType_Secret,
		},
		{
			Key:   "SPACES_SECRET_ACCESS_KEY",
			Value: pkg.Getenv("SPACES_SECRET_ACCESS_KEY", ""),
			Type:  godo.AppVariableType_Secret,
		},
		{
			Key:   "REGION",
			Value: region.String(),
		},
		{
			Key:   "AWS_REGION",
			Value: region.String(),
		},
		{
			Key:   "AWS_ACCESS_KEY_ID",
			Value: pkg.Getenv("SPACES_ACCESS_KEY_ID", ""),
			Type:  godo.AppVariableType_Secret,
		},
		{
			Key:   "AWS_SECRET_ACCESS_KEY",
			Value: pkg.Getenv("SPACES_SECRET_ACCESS_KEY", ""),
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

	//cdTaskName := byoc.CdTaskPrefix
	//serviceContainers := []*godo.AppServiceSpec{
	//	{
	//		Name: "main",
	//		Image: &godo.ImageSourceSpec{
	//			Repository:   "pulumi-nodejs",
	//			Registry:     "pulumi",
	//			RegistryType: DockerHub,
	//		},
	//		RunCommand:       "node lib/index.js",
	//		InstanceCount:    1,
	//		InstanceSizeSlug: "basic-xs",
	//	},
	//}
	//jobContainers := []*godo.AppJobSpec{
	//	{
	//		Name: cdTaskName,
	//		Image: &godo.ImageSourceSpec{
	//			Repository:   "cd",
	//			RegistryType: DockerHub,
	//		},
	//		InstanceCount:    1,
	//		InstanceSizeSlug: "basic-xxs",
	//		Kind:             godo.AppJobSpecKind_PreDeploy,
	//	},
	//}
	//
	//if err := b.driver.SetUp(ctx, serviceContainers, jobContainers); err != nil {
	//	return err
	//}

	b.SetupDone = true

	return nil
}
