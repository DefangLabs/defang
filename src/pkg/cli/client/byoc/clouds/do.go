package clouds

import (
	"context"
	"github.com/defang-io/defang/src/pkg/cli/client"
	"github.com/defang-io/defang/src/pkg/clouds/do/appPlatform"
	"github.com/defang-io/defang/src/pkg/quota"
	"github.com/defang-io/defang/src/pkg/types"
	defangv1 "github.com/defang-io/defang/src/protos/io/defang/v1"
	"strings"
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

func (b ByocDo) setUp(ctx context.Context) error {

	return nil
}

func (b ByocDo) Deploy(ctx context.Context, req *defangv1.DeployRequest) (*defangv1.DeployResponse, error) {

	return nil, nil
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
