package client

import (
	"context"
	"errors"
	"io"
	"os"

	"github.com/DefangLabs/defang/src/pkg/dns"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/bufbuild/connect-go"
)

type PlaygroundProvider struct {
	FabricClient
	RetryDelayer
}

var _ Provider = (*PlaygroundProvider)(nil)

func (g *PlaygroundProvider) Deploy(ctx context.Context, req *defangv1.DeployRequest) (*defangv1.DeployResponse, error) {
	if os.Getenv("DEFANG_PULUMI_DIR") != "" {
		return nil, errors.New("DEFANG_PULUMI_DIR is set, but not supported by the Playground provider")
	}
	return getMsg(g.GetController().Deploy(ctx, connect.NewRequest(req)))
}

func (g *PlaygroundProvider) GetDeploymentStatus(ctx context.Context) error {
	return io.EOF // TODO: implement on fabric, for now assume service is deployed
}

func (g *PlaygroundProvider) Preview(ctx context.Context, req *defangv1.DeployRequest) (*defangv1.DeployResponse, error) {
	req.Preview = true
	return g.Deploy(ctx, req)
}

func (g *PlaygroundProvider) Estimate(ctx context.Context, req *defangv1.EstimateRequest) (*defangv1.EstimateResponse, error) {
	return getMsg(g.GetController().Estimate(ctx, connect.NewRequest(req)))
}

func (g *PlaygroundProvider) GetProjectUpdate(context.Context, string) (*defangv1.ProjectUpdate, error) {
	return nil, errors.New("the project update command is not valid for the Defang playground; did you forget --provider?")
}

func (g *PlaygroundProvider) GetService(ctx context.Context, req *defangv1.GetRequest) (*defangv1.ServiceInfo, error) {
	return getMsg(g.GetController().Get(ctx, connect.NewRequest(req)))
}

func (g *PlaygroundProvider) Delete(ctx context.Context, req *defangv1.DeleteRequest) (*defangv1.DeleteResponse, error) {
	return getMsg(g.GetController().Delete(ctx, connect.NewRequest(req)))
}

func (g *PlaygroundProvider) GetServices(ctx context.Context, req *defangv1.GetServicesRequest) (*defangv1.GetServicesResponse, error) {
	return getMsg(g.GetController().GetServices(ctx, connect.NewRequest(req)))
}

func (g *PlaygroundProvider) PutConfig(ctx context.Context, req *defangv1.PutConfigRequest) error {
	_, err := g.GetController().PutSecret(ctx, connect.NewRequest(req))
	return err
}

func (g *PlaygroundProvider) DeleteConfig(ctx context.Context, req *defangv1.Secrets) error {
	_, err := g.GetController().DeleteSecrets(ctx, connect.NewRequest(req))
	return err
}

func (g *PlaygroundProvider) ListConfig(ctx context.Context, req *defangv1.ListConfigsRequest) (*defangv1.Secrets, error) {
	return getMsg(g.GetController().ListSecrets(ctx, connect.NewRequest(req)))
}

func (g *PlaygroundProvider) CreateUploadURL(ctx context.Context, req *defangv1.UploadURLRequest) (*defangv1.UploadURLResponse, error) {
	return getMsg(g.GetController().CreateUploadURL(ctx, connect.NewRequest(req)))
}

func (g *PlaygroundProvider) Subscribe(ctx context.Context, req *defangv1.SubscribeRequest) (ServerStream[defangv1.SubscribeResponse], error) {
	return g.GetController().Subscribe(ctx, connect.NewRequest(req))
}

func (g *PlaygroundProvider) QueryLogs(ctx context.Context, req *defangv1.TailRequest) (ServerStream[defangv1.TailResponse], error) {
	return g.GetController().Tail(ctx, connect.NewRequest(req))
}

func (g *PlaygroundProvider) BootstrapCommand(ctx context.Context, req BootstrapCommandRequest) (types.ETag, error) {
	return "", errors.New("the CD command is not valid for the Defang playground; did you forget --provider?")
}
func (g *PlaygroundProvider) Destroy(ctx context.Context, req *defangv1.DestroyRequest) (types.ETag, error) {
	resp, err := getMsg(g.GetController().Destroy(ctx, connect.NewRequest(req)))
	if err != nil {
		return "", err
	}
	return resp.Etag, nil
}

func (g *PlaygroundProvider) TearDown(ctx context.Context) error {
	return errors.New("the teardown command is not valid for the Defang playground; did you forget --provider?")
}

func (g *PlaygroundProvider) BootstrapList(context.Context) ([]string, error) {
	return nil, errors.New("this command is not valid for the Defang playground; did you forget --provider?")
}

func (g PlaygroundProvider) ServicePrivateDNS(name string) string {
	return string(g.GetTenantName()) + "-" + name
}

func (g PlaygroundProvider) ServicePublicDNS(name string, projectName string) string {
	// TODO: Move this to fabric since we do not know what shard was assigned, placeholder for now
	return dns.SafeLabel(string(g.GetTenantName())) + "-" + dns.SafeLabel(name) + "." + "prod1b" + ".defang.dev"
}

func (g PlaygroundProvider) RemoteProjectName(ctx context.Context) (string, error) {
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
		AccountID: g.GetTenantName().String(),
		Region:    "us-west-2", // Hardcoded for now for prod1 TODO: Probably should be the current tenant shard?
	}, nil
}

func (g *PlaygroundProvider) QueryForDebug(ctx context.Context, req *defangv1.DebugRequest) error {
	return nil
}

func (g *PlaygroundProvider) PrepareDomainDelegation(ctx context.Context, req PrepareDomainDelegationRequest) (*PrepareDomainDelegationResponse, error) {
	return nil, nil // Playground does not support delegate domains
}
func (g *PlaygroundProvider) SetCanIUseConfig(*defangv1.CanIUseResponse) {}
