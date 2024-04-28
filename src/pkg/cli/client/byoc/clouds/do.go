package clouds

import (
	"context"
	"github.com/defang-io/defang/src/pkg/cli/client"
	"github.com/defang-io/defang/src/pkg/clouds/do/appPlatform"
	"github.com/defang-io/defang/src/pkg/quota"
	"github.com/defang-io/defang/src/pkg/types"
	defangv1 "github.com/defang-io/defang/src/protos/io/defang/v1"
	"github.com/digitalocean/godo"
	"strings"
)

const (
	DockerHub = "DOCKER_HUB"
	Docr      = "DOCR"
)

type ByocDo struct {
	*client.GrpcClient

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

	b := &ByocDo{
		GrpcClient:    defClient,
		CustomDomain:  "",
		Driver:        appPlatform.New(CdTaskPrefix, ""),
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

	//etag := pkg.RandomID()

	//serviceInfos := []*defangv1.ServiceInfo{}
	//var warnings Warnings

	resp := &defangv1.DeployResponse{}
	return resp, nil
}

func (b ByocDo) WhoAmI(ctx context.Context) (*defangv1.WhoAmIResponse, error) {

	return nil, nil
}

func (b ByocDo) GetVersion(context.Context) (*defangv1.Version, error) {
	cdVersion := CdImage[strings.LastIndex(CdImage, ":")+1:]
	return &defangv1.Version{Fabric: cdVersion}, nil
}

func (b ByocDo) Get(ctx context.Context, s *defangv1.ServiceID) (*defangv1.ServiceInfo, error) {

	return nil, nil
}

func (b ByocDo) setUp(ctx context.Context) error {
	if b.setupDone {
		return nil
	}
	cdTaskName := CdTaskPrefix
	serviceContainers := []*godo.AppServiceSpec{
		{
			Name: "main",
			Image: &godo.ImageSourceSpec{
				Repository:   "pulumi-nodejs",
				Registry:     "pulumi",
				RegistryType: DockerHub,
			},
			RunCommand:       "node lib/index.js",
			InstanceCount:    1,
			InstanceSizeSlug: "basic-xxs",
		},
	}
	jobContainers := []*godo.AppJobSpec{
		{
			Name: cdTaskName,
			Image: &godo.ImageSourceSpec{
				Repository:   "cd",
				RegistryType: Docr,
			},
			InstanceCount:    1,
			InstanceSizeSlug: "basic-xxs",
			Kind:             godo.AppJobSpecKind_PreDeploy,
		},
	}

	if err := b.Driver.SetUp(ctx, serviceContainers, jobContainers); err != nil {
		return err
	}

	return nil
}
