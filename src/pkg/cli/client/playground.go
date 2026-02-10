package client

import (
	"context"
	"errors"
	"io"
	"iter"
	"os"

	byocState "github.com/DefangLabs/defang/src/pkg/cli/client/byoc/state"
	"github.com/DefangLabs/defang/src/pkg/dns"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/bufbuild/connect-go"
)

type PlaygroundProvider struct {
	FabricClient
	RetryDelayer
	shardDomain string
	Stack       string
}

var _ Provider = (*PlaygroundProvider)(nil)

func NewPlaygroundProvider(fabricClient FabricClient, stack string) *PlaygroundProvider {
	return &PlaygroundProvider{
		FabricClient: fabricClient,
		Stack:        stack,
	}
}

func (g *PlaygroundProvider) GetStackName() string {
	return g.Stack
}

func (g *PlaygroundProvider) Deploy(ctx context.Context, req *DeployRequest) (*defangv1.DeployResponse, error) {
	if os.Getenv("DEFANG_PULUMI_DIR") != "" {
		return nil, errors.New("DEFANG_PULUMI_DIR is set, but not supported by the Playground provider")
	}
	return getMsg(g.GetFabricClient().Deploy(ctx, connect.NewRequest(&req.DeployRequest)))
}

func (g *PlaygroundProvider) GetDeploymentStatus(ctx context.Context) error {
	return io.EOF // TODO: implement on fabric, for now assume service is deployed
}

func (g *PlaygroundProvider) Preview(ctx context.Context, req *DeployRequest) (*defangv1.DeployResponse, error) {
	req.Preview = true
	return g.Deploy(ctx, req)
}

func (g *PlaygroundProvider) Estimate(ctx context.Context, req *defangv1.EstimateRequest) (*defangv1.EstimateResponse, error) {
	return getMsg(g.GetFabricClient().Estimate(ctx, connect.NewRequest(req)))
}

func (g *PlaygroundProvider) GetProjectUpdate(context.Context, string) (*defangv1.ProjectUpdate, error) {
	return nil, errors.New("the project update command is not valid for the Defang playground; did you forget --stack or --provider?")
}

func (g *PlaygroundProvider) GetService(ctx context.Context, req *defangv1.GetRequest) (*defangv1.ServiceInfo, error) {
	return getMsg(g.GetFabricClient().Get(ctx, connect.NewRequest(req)))
}

func (g *PlaygroundProvider) GetServices(ctx context.Context, req *defangv1.GetServicesRequest) (*defangv1.GetServicesResponse, error) {
	return getMsg(g.GetFabricClient().GetServices(ctx, connect.NewRequest(req)))
}

func (g *PlaygroundProvider) PutConfig(ctx context.Context, req *defangv1.PutConfigRequest) error {
	_, err := g.GetFabricClient().PutSecret(ctx, connect.NewRequest(req))
	return err
}

func (g *PlaygroundProvider) DeleteConfig(ctx context.Context, req *defangv1.Secrets) error {
	_, err := g.GetFabricClient().DeleteSecrets(ctx, connect.NewRequest(req))
	return err
}

func (g *PlaygroundProvider) ListConfig(ctx context.Context, req *defangv1.ListConfigsRequest) (*defangv1.Secrets, error) {
	return getMsg(g.GetFabricClient().ListSecrets(ctx, connect.NewRequest(req)))
}

func (g *PlaygroundProvider) CreateUploadURL(ctx context.Context, req *defangv1.UploadURLRequest) (*defangv1.UploadURLResponse, error) {
	return getMsg(g.GetFabricClient().CreateUploadURL(ctx, connect.NewRequest(req)))
}

func (g *PlaygroundProvider) Subscribe(ctx context.Context, req *defangv1.SubscribeRequest) (ServerStream[defangv1.SubscribeResponse], error) {
	return g.GetFabricClient().Subscribe(ctx, connect.NewRequest(req))
}

func (g *PlaygroundProvider) QueryLogs(ctx context.Context, req *defangv1.TailRequest) (ServerStream[defangv1.TailResponse], error) {
	return g.GetFabricClient().Tail(ctx, connect.NewRequest(req))
}

func (g *PlaygroundProvider) CdCommand(ctx context.Context, req CdCommandRequest) (types.ETag, error) {
	if req.Command == CdCommandDestroy {
		return g.destroy(ctx, &defangv1.DestroyRequest{Project: req.Project})
	}
	return "", errors.New("the CD command is not valid for the Defang playground; did you forget --stack or --provider?")
}

func (g *PlaygroundProvider) destroy(ctx context.Context, req *defangv1.DestroyRequest) (types.ETag, error) {
	resp, err := getMsg(g.GetFabricClient().Destroy(ctx, connect.NewRequest(req)))
	if err != nil {
		return "", err
	}
	return resp.Etag, nil
}

func (g *PlaygroundProvider) TearDownCD(ctx context.Context) error {
	return errors.New("the teardown command is not valid for the Defang playground; did you forget --stack or --provider?")
}

func (g *PlaygroundProvider) SetUpCD(ctx context.Context) error {
	return errors.New("this command is not valid for the Defang playground; did you forget --stack or --provider?")
}

func (g *PlaygroundProvider) CdList(context.Context, bool) (iter.Seq[*byocState.Info], error) {
	return nil, errors.New("this command is not valid for the Defang playground; did you forget --stack or --provider?")
}

func (g *PlaygroundProvider) ServicePrivateDNS(name string) string {
	return dns.SafeLabel(string(g.GetTenantName()) + "-" + name)
}

func (g *PlaygroundProvider) UpdateShardDomain(ctx context.Context) error {
	resp, err := g.GetPlaygroundProjectDomain(ctx)
	if err != nil {
		return err
	}
	g.shardDomain = resp.Domain
	return nil
}

func (g *PlaygroundProvider) ServicePublicDNS(name string, projectName string) string {
	return g.ServicePrivateDNS(name) + "." + g.shardDomain
}

func (g *PlaygroundProvider) RemoteProjectName(ctx context.Context) (string, error) {
	// Hack: Use GetServices to get the current project name
	resp, err := g.GetServices(ctx, &defangv1.GetServicesRequest{})
	if err != nil {
		return "", err
	}
	if resp.Project == "" {
		return "", errors.New("no Playground projects found")
	}
	term.Debug("Using default Playground project: ", resp.Project)
	return resp.Project, nil
}

func (g *PlaygroundProvider) AccountInfo(ctx context.Context) (*AccountInfo, error) {
	return &AccountInfo{
		Provider:  ProviderDefang,
		AccountID: string(g.GetTenantName()),
		Region:    "us-west-2", // Hardcoded for now for prod1 TODO: Probably should be the current tenant shard?
	}, nil
}

func (g *PlaygroundProvider) PrepareDomainDelegation(ctx context.Context, req PrepareDomainDelegationRequest) (*PrepareDomainDelegationResponse, error) {
	return nil, nil // Playground does not support delegate domains
}

func (g *PlaygroundProvider) SetCanIUseConfig(*defangv1.CanIUseResponse) {}

func (PlaygroundProvider) GetStackNameForDomain() string {
	return ""
}
