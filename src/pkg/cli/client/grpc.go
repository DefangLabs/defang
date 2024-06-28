package client

import (
	"context"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/DefangLabs/defang/src/pkg/auth"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/DefangLabs/defang/src/protos/io/defang/v1/defangv1connect"
	"github.com/bufbuild/connect-go"
	"google.golang.org/protobuf/types/known/emptypb"
)

type GrpcClient struct {
	anonID string
	client defangv1connect.FabricControllerClient

	TenantID types.TenantID
	Loader   ProjectLoader
}

func NewGrpcClient(host, accessToken string, tenantID types.TenantID, loader ProjectLoader) GrpcClient {
	baseUrl := "http://"
	if strings.HasSuffix(host, ":443") {
		baseUrl = "https://"
	}
	baseUrl += host
	// Debug(" - Connecting to", baseUrl)
	fabricClient := defangv1connect.NewFabricControllerClient(http.DefaultClient, baseUrl, connect.WithGRPC(), connect.WithInterceptors(auth.NewAuthInterceptor(accessToken), Retrier{}))

	return GrpcClient{client: fabricClient, anonID: GetAnonID(), TenantID: tenantID, Loader: loader}
}

func getMsg[T any](resp *connect.Response[T], err error) (*T, error) {
	if err != nil {
		return nil, err
	}
	return resp.Msg, nil
}

func (g GrpcClient) GetVersions(ctx context.Context) (*defangv1.Version, error) {
	return getMsg(g.client.GetVersion(ctx, &connect.Request[emptypb.Empty]{}))
}

func (g GrpcClient) Token(ctx context.Context, req *defangv1.TokenRequest) (*defangv1.TokenResponse, error) {
	req.AnonId = g.anonID
	return getMsg(g.client.Token(ctx, connect.NewRequest(req)))
}

func (g GrpcClient) RevokeToken(ctx context.Context) error {
	_, err := g.client.RevokeToken(ctx, &connect.Request[emptypb.Empty]{})
	return err
}

func (g GrpcClient) Publish(ctx context.Context, req *defangv1.PublishRequest) error {
	_, err := g.client.Publish(ctx, connect.NewRequest(req))
	return err
}

func (g GrpcClient) GenerateFiles(ctx context.Context, req *defangv1.GenerateFilesRequest) (*defangv1.GenerateFilesResponse, error) {
	return getMsg(g.client.GenerateFiles(ctx, connect.NewRequest(req)))
}

func (g GrpcClient) WhoAmI(ctx context.Context) (*defangv1.WhoAmIResponse, error) {
	return getMsg(g.client.WhoAmI(ctx, &connect.Request[emptypb.Empty]{}))
}

func (g GrpcClient) DelegateSubdomainZone(ctx context.Context, req *defangv1.DelegateSubdomainZoneRequest) (*defangv1.DelegateSubdomainZoneResponse, error) {
	return getMsg(g.client.DelegateSubdomainZone(ctx, connect.NewRequest(req)))
}

func (g GrpcClient) DeleteSubdomainZone(ctx context.Context) error {
	_, err := getMsg(g.client.DeleteSubdomainZone(ctx, &connect.Request[emptypb.Empty]{}))
	return err
}

func (g GrpcClient) GetDelegateSubdomainZone(ctx context.Context) (*defangv1.DelegateSubdomainZoneResponse, error) {
	return getMsg(g.client.GetDelegateSubdomainZone(ctx, &connect.Request[emptypb.Empty]{}))
}

func (g GrpcClient) AgreeToS(ctx context.Context) error {
	_, err := g.client.SignEULA(ctx, &connect.Request[emptypb.Empty]{})
	return err
}

func (g GrpcClient) Track(event string, properties ...Property) error {
	// Convert map[string]any to map[string]string
	var props map[string]string
	if len(properties) > 0 {
		props = make(map[string]string, len(properties))
		for _, p := range properties {
			props[p.Name] = fmt.Sprint(p.Value)
		}
	}
	context, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := g.client.Track(context, &connect.Request[defangv1.TrackRequest]{Msg: &defangv1.TrackRequest{
		AnonId:     g.anonID,
		Event:      event,
		Properties: props,
		Os:         runtime.GOOS,
		Arch:       runtime.GOARCH,
	}})
	return err
}

func (g GrpcClient) CheckLoginAndToS(ctx context.Context) error {
	_, err := g.client.CheckToS(ctx, &connect.Request[emptypb.Empty]{})
	return err
}
