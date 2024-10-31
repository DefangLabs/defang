package client

import (
	"context"
	"errors"

	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/bufbuild/connect-go"
	composeTypes "github.com/compose-spec/compose-go/v2/types"
	"google.golang.org/protobuf/types/known/emptypb"
)

type PlaygroundProvider struct {
	GrpcClient
	project     *composeTypes.Project
	projectName string
}

func (g PlaygroundProvider) LoadProject(ctx context.Context) (*composeTypes.Project, error) {
	if g.project != nil {
		return g.project, nil
	}

	return g.Loader.LoadProject(ctx)
}

func (g PlaygroundProvider) Deploy(ctx context.Context, req *defangv1.DeployRequest) (*defangv1.DeployResponse, error) {
	return getMsg(g.client.Deploy(ctx, connect.NewRequest(req)))
}

func (g PlaygroundProvider) Preview(ctx context.Context, req *defangv1.DeployRequest) (*defangv1.DeployResponse, error) {
	return nil, errors.New("the preview command is not valid for the Defang playground; did you forget --provider?")
}

func (g PlaygroundProvider) GetService(ctx context.Context, req *defangv1.ServiceID) (*defangv1.ServiceInfo, error) {
	return getMsg(g.client.Get(ctx, connect.NewRequest(req)))
}

func (g PlaygroundProvider) Delete(ctx context.Context, req *defangv1.DeleteRequest) (*defangv1.DeleteResponse, error) {
	return getMsg(g.client.Delete(ctx, connect.NewRequest(req)))
}

func (g PlaygroundProvider) GetServices(ctx context.Context) (*defangv1.ListServicesResponse, error) {
	return getMsg(g.client.GetServices(ctx, &connect.Request[emptypb.Empty]{}))
}

func (g PlaygroundProvider) PutConfig(ctx context.Context, req *defangv1.PutConfigRequest) error {
	_, err := g.client.PutSecret(ctx, connect.NewRequest(req))
	return err
}

func (g PlaygroundProvider) DeleteConfig(ctx context.Context, req *defangv1.Secrets) error {
	_, err := g.client.DeleteSecrets(ctx, connect.NewRequest(&defangv1.Secrets{Names: req.Names}))
	return err
}

func (g PlaygroundProvider) ListConfig(ctx context.Context) (*defangv1.Secrets, error) {
	return getMsg(g.client.ListSecrets(ctx, &connect.Request[emptypb.Empty]{}))
}

func (g PlaygroundProvider) CreateUploadURL(ctx context.Context, req *defangv1.UploadURLRequest) (*defangv1.UploadURLResponse, error) {
	return getMsg(g.client.CreateUploadURL(ctx, connect.NewRequest(req)))
}

func (g *PlaygroundProvider) Subscribe(ctx context.Context, req *defangv1.SubscribeRequest) (ServerStream[defangv1.SubscribeResponse], error) {
	return g.client.Subscribe(ctx, connect.NewRequest(req))
}

func (g *PlaygroundProvider) Follow(ctx context.Context, req *defangv1.TailRequest) (ServerStream[defangv1.TailResponse], error) {
	return g.client.Tail(ctx, connect.NewRequest(req))
}

func (g *PlaygroundProvider) BootstrapCommand(ctx context.Context, command string) (types.ETag, error) {
	return "", errors.New("the CD command is not valid for the Defang playground; did you forget --provider?")
}
func (g *PlaygroundProvider) Destroy(ctx context.Context) (types.ETag, error) {
	projectName, err := g.LoadProjectName(ctx)
	if err != nil {
		return "", err
	}

	// Get all the services in the project and delete them all at once
	servicesList, err := g.GetServices(ctx)
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
	resp, err := g.Delete(ctx, &defangv1.DeleteRequest{Project: projectName, Names: names})
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
	return string(g.TenantID) + "-" + name
}

func (g PlaygroundProvider) LoadProjectName(ctx context.Context) (string, error) {
	if g.projectName != "" {
		return g.projectName, nil
	}

	name, err := g.Loader.LoadProjectName(ctx)
	if err == nil {
		return name, nil
	}
	if !errors.Is(err, types.ErrComposeFileNotFound) {
		return "", err
	}

	// Hack: Use GetServices to get the current project name
	// TODO: Use BootstrapList to get the list of projects after playground supports multiple projects
	resp, err := g.GetServices(ctx)
	if err != nil {
		return "", err
	}
	term.Debug("Using default playground project: ", resp.Project)
	return resp.Project, nil
}

func (g *PlaygroundProvider) SetProjectName(projectName string) {
	g.projectName = projectName
}

func (g *PlaygroundProvider) AccountInfo(ctx context.Context) (AccountInfo, error) {
	return PlaygroundAccountInfo{}, nil
}

type PlaygroundAccountInfo struct{}

func (g PlaygroundAccountInfo) AccountID() string { return "playground" }
func (g PlaygroundAccountInfo) Region() string    { return "us-west-2" } // Hardcoded for now for prod1
func (g PlaygroundAccountInfo) Details() string   { return "" }
