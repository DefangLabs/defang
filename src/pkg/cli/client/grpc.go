package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/bufbuild/connect-go"
	"github.com/defang-io/defang/src/pkg/auth"
	v1 "github.com/defang-io/defang/src/protos/io/defang/v1"
	"github.com/defang-io/defang/src/protos/io/defang/v1/defangv1connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/emptypb"
)

type GrpcClient struct {
	anonID string
	client defangv1connect.FabricControllerClient
}

func NewGrpcClient(host, accessToken string) *GrpcClient {
	baseUrl := "http://"
	if strings.HasSuffix(host, ":443") {
		baseUrl = "https://"
	}
	baseUrl += host
	// Debug(" - Connecting to", baseUrl)
	fabricClient := defangv1connect.NewFabricControllerClient(http.DefaultClient, baseUrl, connect.WithGRPC(), connect.WithInterceptors(auth.NewAuthInterceptor(accessToken)))

	state := State{AnonID: uuid.NewString()}

	// Restore anonID from config file
	statePath := path.Join(StateDir, "state.json")
	if bytes, err := os.ReadFile(statePath); err == nil {
		json.Unmarshal(bytes, &state)
	} else { // could be not found or path error
		if bytes, err := json.MarshalIndent(state, "", "  "); err == nil {
			os.MkdirAll(StateDir, 0700)
			os.WriteFile(statePath, bytes, 0644)
		}
	}

	return &GrpcClient{client: fabricClient, anonID: state.AnonID}
}

func getMsg[T any](resp *connect.Response[T], err error) (*T, error) {
	if err != nil {
		return nil, err
	}
	return resp.Msg, nil
}

func (g GrpcClient) GetVersion(ctx context.Context) (*v1.Version, error) {
	return getMsg(g.client.GetVersion(ctx, &connect.Request[emptypb.Empty]{}))
}

func (g GrpcClient) Token(ctx context.Context, req *v1.TokenRequest) (*v1.TokenResponse, error) {
	return getMsg(g.client.Token(ctx, &connect.Request[v1.TokenRequest]{Msg: req}))
}

func (g GrpcClient) RevokeToken(ctx context.Context) error {
	_, err := g.client.RevokeToken(ctx, &connect.Request[emptypb.Empty]{})
	return err
}

func (g GrpcClient) Update(ctx context.Context, req *v1.Service) (*v1.ServiceInfo, error) {
	return getMsg(g.client.Update(ctx, &connect.Request[v1.Service]{Msg: req}))
}

func (g GrpcClient) Deploy(ctx context.Context, req *v1.DeployRequest) (*v1.DeployResponse, error) {
	// return getMsg(g.client.Deploy(ctx, &connect.Request[v1.DeployRequest]{Msg: req})); TODO: implement this
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
	return getMsg(g.client.Get(ctx, &connect.Request[v1.ServiceID]{Msg: req}))
}

func (g GrpcClient) Delete(ctx context.Context, req *v1.DeleteRequest) (*v1.DeleteResponse, error) {
	return getMsg(g.client.Delete(ctx, &connect.Request[v1.DeleteRequest]{Msg: req}))
}

func (g GrpcClient) Publish(ctx context.Context, req *v1.PublishRequest) error {
	_, err := g.client.Publish(ctx, &connect.Request[v1.PublishRequest]{Msg: req})
	return err
}

func (g GrpcClient) GetServices(ctx context.Context) (*v1.ListServicesResponse, error) {
	return getMsg(g.client.GetServices(ctx, &connect.Request[emptypb.Empty]{}))
}

func (g GrpcClient) GenerateFiles(ctx context.Context, req *v1.GenerateFilesRequest) (*v1.GenerateFilesResponse, error) {
	return getMsg(g.client.GenerateFiles(ctx, &connect.Request[v1.GenerateFilesRequest]{Msg: req}))
}

func (g GrpcClient) PutSecret(ctx context.Context, req *v1.SecretValue) error {
	_, err := g.client.PutSecret(ctx, &connect.Request[v1.SecretValue]{Msg: req})
	return err
}

func (g GrpcClient) ListSecrets(ctx context.Context) (*v1.Secrets, error) {
	return getMsg(g.client.ListSecrets(ctx, &connect.Request[emptypb.Empty]{}))
}

func (g GrpcClient) CreateUploadURL(ctx context.Context, req *v1.UploadURLRequest) (*v1.UploadURLResponse, error) {
	return getMsg(g.client.CreateUploadURL(ctx, &connect.Request[v1.UploadURLRequest]{Msg: req}))
}

func (g GrpcClient) WhoAmI(ctx context.Context) (*v1.WhoAmIResponse, error) {
	return getMsg(g.client.WhoAmI(ctx, &connect.Request[emptypb.Empty]{}))
}

func (g GrpcClient) DelegateSubdomainZone(ctx context.Context, req *v1.DelegateSubdomainZoneRequest) (*v1.DelegateSubdomainZoneResponse, error) {
	return getMsg(g.client.DelegateSubdomainZone(ctx, &connect.Request[v1.DelegateSubdomainZoneRequest]{Msg: req}))
}

func (g GrpcClient) DeleteSubdomainZone(ctx context.Context) error {
	_, err := getMsg(g.client.DeleteSubdomainZone(ctx, &connect.Request[emptypb.Empty]{}))
	return err
}

func (g GrpcClient) GetDelegateSubdomainZone(ctx context.Context) (*v1.DelegateSubdomainZoneResponse, error) {
	return getMsg(g.client.GetDelegateSubdomainZone(ctx, &connect.Request[emptypb.Empty]{}))
}

func (g *GrpcClient) Tail(ctx context.Context, req *v1.TailRequest) (ServerStream[v1.TailResponse], error) {
	return g.client.Tail(ctx, &connect.Request[v1.TailRequest]{Msg: req})
}

func (g *GrpcClient) BootstrapCommand(ctx context.Context, command string) (ETag, error) {
	return "", errors.New("the bootstrap command is not valid for the Defang provider")
}

func (g *GrpcClient) AgreeToS(ctx context.Context) error {
	_, err := g.client.SignEULA(ctx, &connect.Request[emptypb.Empty]{})
	return err
}

func (g *GrpcClient) Track(event string, properties ...Property) error {
	// Convert map[string]any to map[string]string
	var props map[string]string
	if len(properties) > 0 {
		props = make(map[string]string, len(properties))
		for _, p := range properties {
			props[p.Name] = fmt.Sprint(p.Value)
		}
	}
	_, err := g.client.Track(context.Background(), &connect.Request[v1.TrackRequest]{Msg: &v1.TrackRequest{
		AnonId:     g.anonID,
		Event:      event,
		Properties: props,
	}})
	return err
}

func (g *GrpcClient) CheckLogin(ctx context.Context) error {
	_, err := g.WhoAmI(ctx)
	return err
}

func (g *GrpcClient) Destroy(ctx context.Context) (ETag, error) {
	// Get all the services and delete them all at once
	services, err := g.GetServices(ctx)
	if err != nil {
		return "", err
	}
	var names []string
	for _, service := range services.Services {
		names = append(names, service.Service.Name)
	}
	resp, err := g.Delete(ctx, &v1.DeleteRequest{Names: names})
	if err != nil {
		return "", err
	}
	return resp.Etag, nil
}

func (g *GrpcClient) TearDown(ctx context.Context) error {
	return errors.New("the teardown command is not valid for the Defang provider")
}
