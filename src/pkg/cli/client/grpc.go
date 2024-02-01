package client

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/bufbuild/connect-go"
	connect_go "github.com/bufbuild/connect-go"
	"github.com/defang-io/defang/src/pkg/auth"
	v1 "github.com/defang-io/defang/src/protos/io/defang/v1"
	"github.com/defang-io/defang/src/protos/io/defang/v1/defangv1connect"
	"google.golang.org/protobuf/types/known/emptypb"
)

type GrpcClient struct {
	client      defangv1connect.FabricControllerClient
	fabric      string
	accessToken string
}

func NewGrpcClient(host, accessToken string) *GrpcClient {
	baseUrl := "http://"
	if strings.HasSuffix(host, ":443") {
		baseUrl = "https://"
	}
	baseUrl += host
	// Debug(" - Connecting to", baseUrl)
	fabricClient := defangv1connect.NewFabricControllerClient(http.DefaultClient, baseUrl, connect.WithGRPC(), connect.WithInterceptors(auth.NewAuthInterceptor(accessToken)))
	return &GrpcClient{client: fabricClient, fabric: host, accessToken: accessToken}
}

func (g GrpcClient) GetFabric() string {
	return g.fabric
}

func (g GrpcClient) GetAccessToken() string {
	return g.accessToken
}

func getMsg[T any](resp *connect_go.Response[T], err error) (*T, error) {
	if err != nil {
		return nil, err
	}
	return resp.Msg, nil
}

func (g GrpcClient) GetStatus(ctx context.Context) (*v1.Status, error) {
	return getMsg(g.client.GetStatus(ctx, &connect_go.Request[emptypb.Empty]{}))
}

func (g GrpcClient) GetVersion(ctx context.Context) (*v1.Version, error) {
	return getMsg(g.client.GetVersion(ctx, &connect_go.Request[emptypb.Empty]{}))
}

func (g GrpcClient) Token(ctx context.Context, req *v1.TokenRequest) (*v1.TokenResponse, error) {
	return getMsg(g.client.Token(ctx, &connect_go.Request[v1.TokenRequest]{Msg: req}))
}

func (g GrpcClient) RevokeToken(ctx context.Context) error {
	_, err := g.client.RevokeToken(ctx, &connect_go.Request[emptypb.Empty]{})
	return err
}

func (g GrpcClient) Update(ctx context.Context, req *v1.Service) (*v1.ServiceInfo, error) {
	return getMsg(g.client.Update(ctx, &connect_go.Request[v1.Service]{Msg: req}))
}

func (g GrpcClient) Deploy(ctx context.Context, req *v1.DeployRequest) (*v1.DeployResponse, error) {
	// return getMsg(g.client.Deploy(ctx, &connect_go.Request[v1.DeployRequest]{Msg: req})); TODO: implement this
	var serviceInfos []*v1.ServiceInfo
	for _, service := range req.Services {
		// Info(" * Publishing service update for", service.Name)
		serviceInfo, err := g.Update(ctx, service)
		if err != nil {
			if len(serviceInfos) == 0 {
				return nil, err // abort if the first service update fails
			}
			// Warn(" ! Failed to update service", service.Name, err)
			continue
		}

		serviceInfos = append(serviceInfos, serviceInfo)
	}
	return &v1.DeployResponse{Services: serviceInfos}, nil
}

func (g GrpcClient) Get(ctx context.Context, req *v1.ServiceID) (*v1.ServiceInfo, error) {
	return getMsg(g.client.Get(ctx, &connect_go.Request[v1.ServiceID]{Msg: req}))
}

func (g GrpcClient) Delete(ctx context.Context, req *v1.DeleteRequest) (*v1.DeleteResponse, error) {
	return getMsg(g.client.Delete(ctx, &connect_go.Request[v1.DeleteRequest]{Msg: req}))
}

func (g GrpcClient) Publish(ctx context.Context, req *v1.PublishRequest) error {
	_, err := g.client.Publish(ctx, &connect_go.Request[v1.PublishRequest]{Msg: req})
	return err
}

func (g GrpcClient) GetServices(ctx context.Context) (*v1.ListServicesResponse, error) {
	return getMsg(g.client.GetServices(ctx, &connect_go.Request[emptypb.Empty]{}))
}

func (g GrpcClient) GenerateFiles(ctx context.Context, req *v1.GenerateFilesRequest) (*v1.GenerateFilesResponse, error) {
	return getMsg(g.client.GenerateFiles(ctx, &connect_go.Request[v1.GenerateFilesRequest]{Msg: req}))
}

func (g GrpcClient) PutSecret(ctx context.Context, req *v1.SecretValue) error {
	_, err := g.client.PutSecret(ctx, &connect_go.Request[v1.SecretValue]{Msg: req})
	return err
}

func (g GrpcClient) ListSecrets(ctx context.Context) (*v1.Secrets, error) {
	return getMsg(g.client.ListSecrets(ctx, &connect_go.Request[emptypb.Empty]{}))
}

func (g GrpcClient) CreateUploadURL(ctx context.Context, req *v1.UploadURLRequest) (*v1.UploadURLResponse, error) {
	return getMsg(g.client.CreateUploadURL(ctx, &connect_go.Request[v1.UploadURLRequest]{Msg: req}))
}

func (g GrpcClient) WhoAmI(ctx context.Context) (*v1.WhoAmIResponse, error) {
	tenant, err := TenantFromAccessToken(g.accessToken)
	if err != nil {
		return nil, err
	}
	return &v1.WhoAmIResponse{Tenant: string(tenant), Account: "defang", Region: "us-west-2"}, nil
	// return getMsg(g.client.WhoAmI(ctx, &connect_go.Request[emptypb.Empty]{})); TODO: implement this rpc
}

func (g *GrpcClient) Tail(ctx context.Context, req *v1.TailRequest) (ServerStream[v1.TailResponse], error) {
	return g.client.Tail(ctx, &connect_go.Request[v1.TailRequest]{Msg: req})
}

func (g *GrpcClient) BootstrapCommand(ctx context.Context, command string) error {
	return errors.New("not a BYOC cluster")
}

func (g *GrpcClient) AgreeToS(ctx context.Context) error {
	_, err := g.client.SignEULA(ctx, &connect_go.Request[emptypb.Empty]{})
	return err
}
