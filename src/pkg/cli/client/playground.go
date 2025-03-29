package client

import (
	"context"
	"errors"

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
	return getMsg(g.GetController().Deploy(ctx, connect.NewRequest(req)))
}

func (g *PlaygroundProvider) GetDeploymentStatus(ctx context.Context) error {
	return errors.New("deployment succeeded") // TODO: implement on fabric, for now assume service is deployed
}

func (g *PlaygroundProvider) Preview(ctx context.Context, req *defangv1.DeployRequest) (*defangv1.DeployResponse, error) {
	return nil, errors.New("the preview command is not valid for the Defang playground; did you forget --provider?")
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
	_, err := g.GetController().DeleteSecrets(ctx, connect.NewRequest(&defangv1.Secrets{Names: req.Names}))
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
	// Get all the services in the project and delete them all at once
	servicesList, err := g.GetServices(ctx, &defangv1.GetServicesRequest{Project: req.Project})
	if err != nil {
		return "", err
	}
	if len(servicesList.Services) == 0 {
		return "", errors.New("no services found")
	}
	var names []string
	for _, service := range servicesList.Services {
		names = append(names, service.Service.Name)
	}

	// FIXME: use Destroy rpc instead of Delete rpc
	resp, err := g.Delete(ctx, &defangv1.DeleteRequest{Project: req.Project, Names: names})
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

func (g PlaygroundProvider) ServiceDNS(name string) string {
	return string(g.GetTenantName()) + "-" + name
}

func (g PlaygroundProvider) RemoteProjectName(ctx context.Context) (string, error) {
	// Hack: Use GetServices to get the current project name
	resp, err := g.GetServices(ctx, &defangv1.GetServicesRequest{})
	if err != nil {
		return "", err
	}
	if resp.Project == "" {
		return "", errors.New("no projects found")
	}
	term.Debug("Using default playground project: ", resp.Project)
	return resp.Project, nil
}

func (g *PlaygroundProvider) AccountInfo(ctx context.Context) (AccountInfo, error) {
	return PlaygroundAccountInfo{}, nil
}

func (g *PlaygroundProvider) QueryForDebug(ctx context.Context, req *defangv1.DebugRequest) error {
	return nil
}

func (g *PlaygroundProvider) PrepareDomainDelegation(ctx context.Context, req PrepareDomainDelegationRequest) (*PrepareDomainDelegationResponse, error) {
	return nil, nil // Playground does not support delegate domains
}
func (g *PlaygroundProvider) SetCanIUseConfig(*defangv1.CanIUseResponse) {}

type PlaygroundAccountInfo struct{}

func (g PlaygroundAccountInfo) AccountID() string    { return "" }
func (g PlaygroundAccountInfo) Details() string      { return "" }
func (g PlaygroundAccountInfo) Provider() ProviderID { return ProviderDefang }
func (g PlaygroundAccountInfo) Region() string       { return "us-west-2" } // Hardcoded for now for prod1
