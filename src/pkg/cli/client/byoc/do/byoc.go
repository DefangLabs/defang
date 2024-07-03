package do

import (
	"bytes"
	"context"
	"fmt"
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
	DockerHub = "DOCKER_HUB"
	Docr      = "DOCR"
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

	//appID, err := b.runCdCommand(ctx, "up", payloadString)
	//if err != nil {
	//	return nil, err
	//}
	//b.AppIds[etag] = appID

	//return &defangv1.DeployResponse{
	//	Services: serviceInfos,
	//	Etag:     etag,
	//}, nil

	res := &defangv1.DeployResponse{}
	return res, nil
}

func (b *ByocDo) BootstrapCommand(ctx context.Context, command string) (string, error) {

	return "", client.ErrNotImplemented("not implemented for ByocDo")
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
	return nil, client.ErrNotImplemented("not implemented for ByocDo")
}

func (b *ByocDo) Destroy(ctx context.Context) (string, error) {
	return b.BootstrapCommand(ctx, "down")
}

func (b *ByocDo) DeleteConfig(ctx context.Context, secrets *defangv1.Secrets) error {
	return client.ErrNotImplemented("not implemented for ByocDo")
}

func (b *ByocDo) GetService(ctx context.Context, s *defangv1.ServiceID) (*defangv1.ServiceInfo, error) {
	return nil, client.ErrNotImplemented("not implemented for ByocDo")
}

func (b *ByocDo) GetServices(ctx context.Context) (*defangv1.ListServicesResponse, error) {
	return nil, client.ErrNotImplemented("not implemented for ByocDo")
}

func (b *ByocDo) ListConfig(ctx context.Context) (*defangv1.Secrets, error) {
	return nil, client.ErrNotImplemented("not implemented for ByocDo")
}

func (b *ByocDo) PutConfig(ctx context.Context, secret *defangv1.SecretValue) error {
	return client.ErrNotImplemented("not implemented for ByocDo")
}

func (b *ByocDo) Restart(ctx context.Context, names ...string) error {
	return client.ErrNotImplemented("not implemented for ByocDo")
}

func (b *ByocDo) ServiceDNS(name string) string {
	return ""
}

func (b *ByocDo) Subscribe(ctx context.Context, req *defangv1.SubscribeRequest) (client.ServerStream[defangv1.SubscribeResponse], error) {
	return nil, client.ErrNotImplemented("not implemented for ByocDo")
}

func (b *ByocDo) Tail(ctx context.Context, req *defangv1.TailRequest) (client.ServerStream[defangv1.TailResponse], error) {
	return nil, client.ErrNotImplemented("not implemented for ByocDo")
}

func (b *ByocDo) TearDown(ctx context.Context) error {
	return client.ErrNotImplemented("not implemented for ByocDo")
	//return b.Driver.TearDown(ctx)
}

func (b *ByocDo) WhoAmI(ctx context.Context) (*defangv1.WhoAmIResponse, error) {
	return nil, client.ErrNotImplemented("not implemented for ByocDo")
}

func (b *ByocDo) GetVersion(context.Context) (*defangv1.Version, error) {
	cdVersion := byoc.CdImage[strings.LastIndex(byoc.CdImage, ":")+1:]
	return &defangv1.Version{Fabric: cdVersion}, nil
}

func (b *ByocDo) Get(ctx context.Context, s *defangv1.ServiceID) (*defangv1.ServiceInfo, error) {
	return nil, client.ErrNotImplemented("not implemented for ByocDo")
}

func (b *ByocDo) runCdCommand(ctx context.Context, cmd ...string) (string, error) {
	env := b.environment()
	if term.DoDebug() {
		debugEnv := " -"
		for k, v := range env {
			debugEnv += " " + k + "=" + v
		}
		term.Debug(debugEnv, "npm run dev", strings.Join(cmd, " "))
	}
	return b.driver.Run(ctx, env, cmd...)
}

func (b *ByocDo) environment() map[string]string {
	region := b.driver.Region // TODO: this should be the destination region, not the CD region; make customizable
	return map[string]string{
		// "AWS_REGION":               region.String(), should be set by ECS (because of CD task role)
		"DEFANG_PREFIX":              byoc.DefangPrefix,
		"DEFANG_DEBUG":               os.Getenv("DEFANG_DEBUG"), // TODO: use the global DoDebug flag
		"DEFANG_ORG":                 b.TenantID,
		"DOMAIN":                     b.ProjectDomain,
		"PRIVATE_DOMAIN":             b.PrivateDomain,
		"PROJECT":                    b.PulumiProject,
		"PULUMI_BACKEND_URL":         fmt.Sprintf(`s3://%s.digitaloceanspaces.com/%s`, region, b.driver.BucketName), // TODO: add a way to override bucket
		"PULUMI_CONFIG_PASSPHRASE":   pkg.Getenv("PULUMI_CONFIG_PASSPHRASE", "asdf"),                                // TODO: make customizable
		"STACK":                      b.PulumiStack,
		"NPM_CONFIG_UPDATE_NOTIFIER": "false",
		"PULUMI_SKIP_UPDATE_CHECK":   "true",
		"DO_PAT":                     os.Getenv("DO_PAT"),
		"DO_SPACES_ID":               os.Getenv("DO_SPACES_ID"),
		"DO_SPACES_KEY":              os.Getenv("DO_SPACES_KEY"),
	}
}

func (b *ByocDo) update(ctx context.Context, service *defangv1.Service) (*defangv1.ServiceInfo, error) {

	si := &defangv1.ServiceInfo{
		Service: &defangv1.ServiceID{Name: service.Name},
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

	//if err := b.Driver.SetUp(ctx, serviceContainers, jobContainers); err != nil {
	//	return err
	//}

	b.SetupDone = true

	return nil
}
