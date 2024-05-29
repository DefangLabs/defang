package client

import (
	"context"
	"errors"

	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/bufbuild/connect-go"
	compose "github.com/compose-spec/compose-go/v2/types"
	"google.golang.org/protobuf/types/known/emptypb"
)

type PlaygroundClient struct {
	GrpcClient
}

func (g PlaygroundClient) LoadProject() (*compose.Project, error) {
	projectName, _ := g.LoadProjectName()
	return g.Loader.LoadWithDefaultProjectName(projectName)
}

func (g PlaygroundClient) Update(ctx context.Context, req *defangv1.Service) (*defangv1.ServiceInfo, error) {
	return getMsg(g.client.Update(ctx, connect.NewRequest(req)))
}

func (g PlaygroundClient) Deploy(ctx context.Context, req *defangv1.DeployRequest) (*defangv1.DeployResponse, error) {
	return getMsg(g.client.Deploy(ctx, connect.NewRequest(req)))
}

func (g PlaygroundClient) GetService(ctx context.Context, req *defangv1.ServiceID) (*defangv1.ServiceInfo, error) {
	return getMsg(g.client.Get(ctx, connect.NewRequest(req)))
}

func (g PlaygroundClient) Delete(ctx context.Context, req *defangv1.DeleteRequest) (*defangv1.DeleteResponse, error) {
	return getMsg(g.client.Delete(ctx, connect.NewRequest(req)))
}

func (g PlaygroundClient) GetServices(ctx context.Context) (*defangv1.ListServicesResponse, error) {
	return getMsg(g.client.GetServices(ctx, &connect.Request[emptypb.Empty]{}))
}

func (g PlaygroundClient) PutConfig(ctx context.Context, req *defangv1.SecretValue) error {
	_, err := g.client.PutSecret(ctx, connect.NewRequest(req))
	return err
}

func (g PlaygroundClient) DeleteConfig(ctx context.Context, req *defangv1.Secrets) error {
	_, err := g.client.DeleteSecrets(ctx, connect.NewRequest(&defangv1.Secrets{Names: req.Names}))
	return err
}

func (g PlaygroundClient) ListConfig(ctx context.Context) (*defangv1.Secrets, error) {
	return getMsg(g.client.ListSecrets(ctx, &connect.Request[emptypb.Empty]{}))
}

func (g PlaygroundClient) CreateUploadURL(ctx context.Context, req *defangv1.UploadURLRequest) (*defangv1.UploadURLResponse, error) {
	return getMsg(g.client.CreateUploadURL(ctx, connect.NewRequest(req)))
}

func (g *PlaygroundClient) Tail(ctx context.Context, req *defangv1.TailRequest) (ServerStream[defangv1.TailResponse], error) {
	return g.client.Tail(ctx, connect.NewRequest(req))
}

func (g *PlaygroundClient) BootstrapCommand(ctx context.Context, command string) (types.ETag, error) {
	return "", errors.New("the bootstrap command is not valid for the Defang playground; did you forget --provider?")
}
func (g *PlaygroundClient) Destroy(ctx context.Context) (types.ETag, error) {
	// Get all the services in the project and delete them all at once
	project, err := g.GetServices(ctx)
	if err != nil {
		return "", err
	}
	if len(project.Services) == 0 {
		return "", errors.New("no services found")
	}
	var names []string
	for _, service := range project.Services {
		names = append(names, service.Service.Name)
	}
	resp, err := g.Delete(ctx, &defangv1.DeleteRequest{Names: names})
	if err != nil {
		return "", err
	}
	return resp.Etag, nil
}

func (g *PlaygroundClient) TearDown(ctx context.Context) error {
	return errors.New("the teardown command is not valid for the Defang playground; did you forget --provider?")
}

func (g *PlaygroundClient) BootstrapList(context.Context) ([]string, error) {
	return nil, errors.New("this command is not valid for the Defang playground; did you forget --provider?")
}

func (g *PlaygroundClient) Restart(ctx context.Context, names ...string) (types.ETag, error) {
	// For now, we'll just get the service info and pass it back to Deploy as-is.
	services := make([]*defangv1.Service, 0, len(names))
	for _, name := range names {
		serviceInfo, err := g.GetService(ctx, &defangv1.ServiceID{Name: name})
		if err != nil {
			return "", err
		}
		services = append(services, serviceInfo.Service)
	}

	dr, err := g.Deploy(ctx, &defangv1.DeployRequest{Services: services})
	if err != nil {
		return "", err
	}
	return dr.Etag, nil
}

func (g PlaygroundClient) ServiceDNS(name string) string {
	return string(g.tenantID) + "-" + name
}

func (g PlaygroundClient) LoadProjectName() (string, error) {
	return string(g.tenantID), nil
}
