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
	Client defangv1connect.FabricControllerClient

	TenantName types.TenantName
}

func NewGrpcClient(host, accessToken string, tenantName types.TenantName) GrpcClient {
	baseUrl := "http://"
	if strings.HasSuffix(host, ":443") {
		baseUrl = "https://"
	}
	baseUrl += host
	// Debug(" - Connecting to", baseUrl)
	fabricClient := defangv1connect.NewFabricControllerClient(http.DefaultClient, baseUrl, connect.WithGRPC(), connect.WithInterceptors(auth.NewAuthInterceptor(accessToken), Retrier{}))

	return GrpcClient{Client: fabricClient, anonID: GetAnonID(), TenantName: tenantName}
}

func getMsg[T any](resp *connect.Response[T], err error) (*T, error) {
	if err != nil {
		return nil, err
	}
	return resp.Msg, nil
}

func (g GrpcClient) GetVersions(ctx context.Context) (*defangv1.Version, error) {
	return getMsg(g.Client.GetVersion(ctx, &connect.Request[emptypb.Empty]{}))
}

func (g GrpcClient) Token(ctx context.Context, req *defangv1.TokenRequest) (*defangv1.TokenResponse, error) {
	req.AnonId = g.anonID
	return getMsg(g.Client.Token(ctx, connect.NewRequest(req)))
}

func (g GrpcClient) RevokeToken(ctx context.Context) error {
	_, err := g.Client.RevokeToken(ctx, &connect.Request[emptypb.Empty]{})
	return err
}

func (g GrpcClient) Publish(ctx context.Context, req *defangv1.PublishRequest) error {
	_, err := g.Client.Publish(ctx, connect.NewRequest(req))
	return err
}

func (g GrpcClient) PutDeployment(ctx context.Context, req *defangv1.PutDeploymentRequest) error {
	_, err := g.Client.PutDeployment(ctx, connect.NewRequest(req))
	return err
}

func (g GrpcClient) ListDeployments(ctx context.Context, req *defangv1.ListDeploymentsRequest) (*defangv1.ListDeploymentsResponse, error) {
	return getMsg(g.Client.ListDeployments(ctx, connect.NewRequest(req)))
}

func (g GrpcClient) GenerateFiles(ctx context.Context, req *defangv1.GenerateFilesRequest) (*defangv1.GenerateFilesResponse, error) {
	return getMsg(g.Client.GenerateFiles(ctx, connect.NewRequest(req)))
}

func (g GrpcClient) WhoAmI(ctx context.Context) (*defangv1.WhoAmIResponse, error) {
	return getMsg(g.Client.WhoAmI(ctx, &connect.Request[emptypb.Empty]{}))
}

func (g GrpcClient) DelegateSubdomainZone(ctx context.Context, req *defangv1.DelegateSubdomainZoneRequest) (*defangv1.DelegateSubdomainZoneResponse, error) {
	return getMsg(g.Client.DelegateSubdomainZone(ctx, connect.NewRequest(req)))
}

func (g GrpcClient) DeleteSubdomainZone(ctx context.Context) error {
	_, err := getMsg(g.Client.DeleteSubdomainZone(ctx, &connect.Request[emptypb.Empty]{}))
	return err
}

func (g GrpcClient) GetDelegateSubdomainZone(ctx context.Context) (*defangv1.DelegateSubdomainZoneResponse, error) {
	return getMsg(g.Client.GetDelegateSubdomainZone(ctx, &connect.Request[emptypb.Empty]{}))
}

func (g GrpcClient) AgreeToS(ctx context.Context) error {
	_, err := g.Client.SignEULA(ctx, &connect.Request[emptypb.Empty]{})
	return err
}

func (g GrpcClient) Debug(ctx context.Context, req *defangv1.DebugRequest) (*defangv1.DebugResponse, error) {
	return getMsg(g.Client.Debug(ctx, connect.NewRequest(req)))
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
	_, err := g.Client.Track(context, &connect.Request[defangv1.TrackRequest]{Msg: &defangv1.TrackRequest{
		AnonId:     g.anonID,
		Event:      event,
		Properties: props,
		Os:         runtime.GOOS,
		Arch:       runtime.GOARCH,
	}})
	return err
}

func (g GrpcClient) CheckLoginAndToS(ctx context.Context) error {
	_, err := g.Client.CheckToS(ctx, &connect.Request[emptypb.Empty]{})
	return err
}

func (g GrpcClient) VerifyDNSSetup(ctx context.Context, req *defangv1.VerifyDNSSetupRequest) error {
	_, err := g.Client.VerifyDNSSetup(ctx, connect.NewRequest(req))
	return err
}

func (g GrpcClient) GetSelectedProvider(ctx context.Context, req *defangv1.GetSelectedProviderRequest) (*defangv1.GetSelectedProviderResponse, error) {
	return getMsg(g.Client.GetSelectedProvider(ctx, connect.NewRequest(req)))
}

func (g GrpcClient) SetSelectedProvider(ctx context.Context, req *defangv1.SetSelectedProviderRequest) error {
	_, err := g.Client.SetSelectedProvider(ctx, connect.NewRequest(req))
	return err
}

func (g GrpcClient) CanIUse(ctx context.Context, req *defangv1.CanIUseRequest) (*defangv1.CanIUseResponse, error) {
	return getMsg(g.Client.CanIUse(ctx, connect.NewRequest(req)))
}
