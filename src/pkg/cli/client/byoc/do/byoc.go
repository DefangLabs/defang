package do

import (
	"bytes"
	"context"
	"fmt"
	"github.com/digitalocean/godo"
	"os"
	"strings"

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

const (
	DockerHub     = "DOCKER_HUB"
	Docr          = "DOCR"
	Secret        = "SECRET"
	CommandPrefix = "node lib/index.js"
)

type ByocDo struct {
	byoc.ByocBaseClient

	appIds map[string]string
	driver *appPlatform.DoApp
}

func NewByoc(grpcClient client.GrpcClient, tenantId types.TenantID) *ByocDo {
	regionString := os.Getenv("REGION")

	if regionString == "" {
		regionString = "sfo3"
	}

	b := &ByocDo{
		ByocBaseClient: *byoc.NewByocBaseClient(grpcClient, tenantId),
		driver:         appPlatform.New(byoc.CdTaskPrefix, do.Region(regionString)),
		appIds:         map[string]string{},
	}

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

	url, err := b.driver.CreateUploadURL(ctx, etag)
	if err != nil {
		return nil, err
	}

	// Do an HTTP PUT to the generated URL
	resp, err := http.Put(ctx, url, "application/protobuf", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("unexpected status code during upload: %s", resp.Status)
	}
	payloadUrl := strings.Split(http.RemoveQueryParam(url), "/")

	payloadFileName := payloadUrl[len(payloadUrl)-1]

	payloadString, err := b.driver.CreateS3DownloadUrl(ctx, fmt.Sprintf("uploads/%s", payloadFileName))

	if err != nil {
		return nil, err
	}

	term.Debug(fmt.Sprintf("PAYLOAD STRING: %s", payloadString))

	//appID, err := b.runCdCommand(ctx, fmt.Sprintf("%s %s %s", CommandPrefix, "up", payloadString))
	//if err != nil {
	//	return nil, err
	//}
	//b.appIds[etag] = appID

	return &defangv1.DeployResponse{
		Services: serviceInfos,
		Etag:     etag,
	}, nil
}

func (b *ByocDo) BootstrapCommand(ctx context.Context, command string) (string, error) {
	if err := b.setUp(ctx); err != nil {
		return "", err
	}

	foo, err := b.runCdCommand(ctx, fmt.Sprintf("%s %s", CommandPrefix, "up"))
	if err != nil {
		return "", err
	}

	return foo, nil
}

func (b *ByocDo) BootstrapList(ctx context.Context) ([]string, error) {
	return nil, nil
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
	return nil, nil
}

func (b *ByocDo) Destroy(ctx context.Context) (string, error) {
	return b.BootstrapCommand(ctx, "down")
}

func (b *ByocDo) DeleteConfig(ctx context.Context, secrets *defangv1.Secrets) error {
	return nil
}

func (b *ByocDo) GetService(ctx context.Context, s *defangv1.ServiceID) (*defangv1.ServiceInfo, error) {
	return nil, nil
}

func (b *ByocDo) GetServices(ctx context.Context) (*defangv1.ListServicesResponse, error) {
	return nil, nil
}

func (b *ByocDo) ListConfig(ctx context.Context) (*defangv1.Secrets, error) {
	return nil, nil
}

func (b *ByocDo) PutConfig(ctx context.Context, secret *defangv1.SecretValue) error {
	return nil
}

func (b *ByocDo) Restart(ctx context.Context, names ...string) (types.ETag, error) {
	return "", nil
}

func (b *ByocDo) ServiceDNS(name string) string {
	return ""
}

func (b *ByocDo) Tail(ctx context.Context, req *defangv1.TailRequest) (client.ServerStream[defangv1.TailResponse], error) {
	if err := b.setUp(ctx); err != nil {
		return nil, err
	}

	client := b.driver.Client

	logs, _, err := client.Apps.GetLogs(ctx, "bb05e4c4-f8c8-4440-a1a0-07f88c353fea", "", "", godo.AppLogType("RUN"), true, 5)
	if err != nil {
		return nil, err
	}
	term.Debug("LIVE URL")
	term.Debug(logs.LiveURL)
	//newByocServerStream(ctx, logs, nil)

	if err != nil {
		return nil, err
	}

	return nil, nil
}

func (b *ByocDo) TearDown(ctx context.Context) error {
	return nil
	//return b.Driver.TearDown(ctx)
}

func (b *ByocDo) WhoAmI(ctx context.Context) (*defangv1.WhoAmIResponse, error) {

	return nil, nil
}

func (b *ByocDo) GetVersion(context.Context) (*defangv1.Version, error) {
	cdVersion := byoc.CdImage[strings.LastIndex(byoc.CdImage, ":")+1:]
	return &defangv1.Version{Fabric: cdVersion}, nil
}

func (b *ByocDo) Get(ctx context.Context, s *defangv1.ServiceID) (*defangv1.ServiceInfo, error) {

	return nil, nil
}

func (b *ByocDo) runCdCommand(ctx context.Context, cmd string) (string, error) {
	env := b.environment()
	return b.driver.Run(ctx, env, cmd)
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
			Type:  Secret,
		},
		{
			Key:   "SPACES_SECRET_ACCESS_KEY",
			Value: pkg.Getenv("SPACES_SECRET_ACCESS_KEY", ""),
			Type:  Secret,
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
			Type:  Secret,
		},
		{
			Key:   "AWS_SECRET_ACCESS_KEY",
			Value: pkg.Getenv("SPACES_SECRET_ACCESS_KEY", ""),
			Type:  Secret,
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
