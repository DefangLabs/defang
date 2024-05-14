package do

import (
	"bytes"
	"context"
	"fmt"
	"github.com/defang-io/defang/src/pkg"
	"github.com/defang-io/defang/src/pkg/cli/client"
	"github.com/defang-io/defang/src/pkg/cli/client/byoc"

	"github.com/defang-io/defang/src/pkg/clouds/do"
	"github.com/defang-io/defang/src/pkg/clouds/do/appPlatform"
	"github.com/defang-io/defang/src/pkg/http"
	"github.com/defang-io/defang/src/pkg/quota"
	"github.com/defang-io/defang/src/pkg/term"
	"github.com/defang-io/defang/src/pkg/types"
	defangv1 "github.com/defang-io/defang/src/protos/io/defang/v1"
	"google.golang.org/protobuf/proto"
	"os"
	"strings"
)

const (
	DockerHub = "DOCKER_HUB"
	Docr      = "DOCR"
)

type ByocDo struct {
	*client.GrpcClient

	AppIds                  map[string]string
	Driver                  *appPlatform.DoApp
	CustomDomain            string
	privateDomain           string
	privateLBIps            []string
	privateNatIps           []string
	pulumiProject           string
	pulumiStack             string
	quota                   quota.Quotas
	setupDone               bool
	TenantID                string
	shouldDelegateSubdomain bool
}

func NewByocDO(tenantId types.TenantID, project string, defClient *client.GrpcClient) *ByocDo {
	if project == "" {
		project = tenantId.String()
	}

	regionString := os.Getenv("REGION")

	if regionString == "" {
		regionString = "sfo3"
	}

	b := &ByocDo{
		GrpcClient:    defClient,
		CustomDomain:  "",
		Driver:        appPlatform.New(byoc.CdTaskPrefix, do.Region(regionString)),
		pulumiProject: project,
		pulumiStack:   "beta",
		TenantID:      tenantId.String(),
	}

	return b
}

func (b ByocDo) Deploy(ctx context.Context, req *defangv1.DeployRequest) (*defangv1.DeployResponse, error) {
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

	url, err := b.Driver.CreateUploadUrl(ctx, etag)
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

	payloadString, err := b.Driver.CreateS3DownloadUrl(ctx, fmt.Sprintf("uploads/%s", payloadFileName))

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

func (b ByocDo) WhoAmI(ctx context.Context) (*defangv1.WhoAmIResponse, error) {

	return nil, nil
}

func (b ByocDo) GetVersion(context.Context) (*defangv1.Version, error) {
	cdVersion := byoc.CdImage[strings.LastIndex(byoc.CdImage, ":")+1:]
	return &defangv1.Version{Fabric: cdVersion}, nil
}

func (b ByocDo) Get(ctx context.Context, s *defangv1.ServiceID) (*defangv1.ServiceInfo, error) {

	return nil, nil
}

func (b ByocDo) runCdCommand(ctx context.Context, cmd ...string) (string, error) {
	env := b.environment()
	if term.DoDebug {
		debugEnv := " -"
		for k, v := range env {
			debugEnv += " " + k + "=" + v
		}
		term.Debug(debugEnv, "npm run dev", strings.Join(cmd, " "))
	}
	return b.Driver.Run(ctx, env, cmd...)
}

func (b ByocDo) environment() map[string]string {
	region := b.Driver.Region // TODO: this should be the destination region, not the CD region; make customizable
	return map[string]string{
		// "AWS_REGION":               region.String(), should be set by ECS (because of CD task role)
		"DEFANG_PREFIX":              byoc.DefangPrefix,
		"DEFANG_DEBUG":               os.Getenv("DEFANG_DEBUG"), // TODO: use the global DoDebug flag
		"DEFANG_ORG":                 b.TenantID,
		"DOMAIN":                     b.CustomDomain,
		"PRIVATE_DOMAIN":             b.privateDomain,
		"PROJECT":                    b.pulumiProject,
		"PULUMI_BACKEND_URL":         fmt.Sprintf(`s3://%s.digitaloceanspaces.com/%s`, region, b.Driver.BucketName), // TODO: add a way to override bucket
		"PULUMI_CONFIG_PASSPHRASE":   pkg.Getenv("PULUMI_CONFIG_PASSPHRASE", "asdf"),                                // TODO: make customizable
		"STACK":                      b.pulumiStack,
		"NPM_CONFIG_UPDATE_NOTIFIER": "false",
		"PULUMI_SKIP_UPDATE_CHECK":   "true",
		"DO_PAT":                     os.Getenv("DO_PAT"),
		"DO_SPACES_ID":               os.Getenv("DO_SPACES_ID"),
		"DO_SPACES_KEY":              os.Getenv("DO_SPACES_KEY"),
	}
}

func (b ByocDo) update(ctx context.Context, service *defangv1.Service) (*defangv1.ServiceInfo, error) {

	si := &defangv1.ServiceInfo{
		Service: service,
		Project: b.pulumiProject,
		Etag:    pkg.RandomID(),
	}

	//hasIngress := false
	//fqn := service.Name
	return si, nil
}

func (b ByocDo) setUp(ctx context.Context) error {
	if b.setupDone {
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

	b.setupDone = true

	return nil
}
