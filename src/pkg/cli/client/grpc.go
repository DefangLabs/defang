package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/bufbuild/connect-go"
	compose "github.com/compose-spec/compose-go/v2/types"
	"github.com/defang-io/defang/src/pkg/auth"
	"github.com/defang-io/defang/src/pkg/term"
	"github.com/defang-io/defang/src/pkg/types"
	defangv1 "github.com/defang-io/defang/src/protos/io/defang/v1"
	"github.com/defang-io/defang/src/protos/io/defang/v1/defangv1connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/emptypb"
)

type GrpcClient struct {
	anonID string
	client defangv1connect.FabricControllerClient

	tenantID types.TenantID
	Loader   ProjectLoader
}

func NewGrpcClient(host, accessToken string, tenantID types.TenantID, loader ProjectLoader) *GrpcClient {
	baseUrl := "http://"
	if strings.HasSuffix(host, ":443") {
		baseUrl = "https://"
	}
	baseUrl += host
	// Debug(" - Connecting to", baseUrl)
	fabricClient := defangv1connect.NewFabricControllerClient(http.DefaultClient, baseUrl, connect.WithGRPC(), connect.WithInterceptors(auth.NewAuthInterceptor(accessToken)))

	state := State{AnonID: uuid.NewString()}

	// Restore anonID from config file
	statePath := filepath.Join(StateDir, "state.json")
	if bytes, err := os.ReadFile(statePath); err == nil {
		json.Unmarshal(bytes, &state)
	} else { // could be not found or path error
		if bytes, err := json.MarshalIndent(state, "", "  "); err == nil {
			os.MkdirAll(StateDir, 0700)
			os.WriteFile(statePath, bytes, 0644)
		}
	}

	return &GrpcClient{client: fabricClient, anonID: state.AnonID, tenantID: tenantID, Loader: loader}
}

func getMsg[T any](resp *connect.Response[T], err error) (*T, error) {
	if err != nil {
		return nil, err
	}
	return resp.Msg, nil
}

func (g GrpcClient) LoadProject() (*compose.Project, error) {
	projectName, _ := g.LoadProjectName()
	return g.Loader.LoadWithDefaultProjectName(projectName)
}

func (g GrpcClient) GetVersions(ctx context.Context) (*defangv1.Version, error) {
	return getMsg(g.client.GetVersion(ctx, &connect.Request[emptypb.Empty]{}))
}

func (g GrpcClient) Token(ctx context.Context, req *defangv1.TokenRequest) (*defangv1.TokenResponse, error) {
	req.AnonId = g.anonID
	return getMsg(g.client.Token(ctx, &connect.Request[defangv1.TokenRequest]{Msg: req}))
}

func (g GrpcClient) RevokeToken(ctx context.Context) error {
	_, err := g.client.RevokeToken(ctx, &connect.Request[emptypb.Empty]{})
	return err
}

func (g GrpcClient) Update(ctx context.Context, req *defangv1.Service) (*defangv1.ServiceInfo, error) {
	return getMsg(g.client.Update(ctx, &connect.Request[defangv1.Service]{Msg: req}))
}

func (g GrpcClient) Deploy(ctx context.Context, req *defangv1.DeployRequest) (*defangv1.DeployResponse, error) {
	// TODO: remove this when playground supports BYOD
	for _, service := range req.Services {
		if service.Domainname != "" {
			term.Warnf("Defang provider does not support the domainname field for now, service: %v, domain: %v", service.Name, service.Domainname)
		}
	}
	return getMsg(g.client.Deploy(ctx, &connect.Request[defangv1.DeployRequest]{Msg: req}))
}

func (g GrpcClient) Get(ctx context.Context, req *defangv1.ServiceID) (*defangv1.ServiceInfo, error) {
	return getMsg(g.client.Get(ctx, &connect.Request[defangv1.ServiceID]{Msg: req}))
}

func (g GrpcClient) Delete(ctx context.Context, req *defangv1.DeleteRequest) (*defangv1.DeleteResponse, error) {
	return getMsg(g.client.Delete(ctx, &connect.Request[defangv1.DeleteRequest]{Msg: req}))
}

func (g GrpcClient) Publish(ctx context.Context, req *defangv1.PublishRequest) error {
	_, err := g.client.Publish(ctx, &connect.Request[defangv1.PublishRequest]{Msg: req})
	return err
}

func (g GrpcClient) GetServices(ctx context.Context) (*defangv1.ListServicesResponse, error) {
	return getMsg(g.client.GetServices(ctx, &connect.Request[emptypb.Empty]{}))
}

func (g GrpcClient) GenerateFiles(ctx context.Context, req *defangv1.GenerateFilesRequest) (*defangv1.GenerateFilesResponse, error) {
	return getMsg(g.client.GenerateFiles(ctx, &connect.Request[defangv1.GenerateFilesRequest]{Msg: req}))
}

func (g GrpcClient) PutConfig(ctx context.Context, req *defangv1.SecretValue) error {
	_, err := g.client.PutSecret(ctx, &connect.Request[defangv1.SecretValue]{Msg: req})
	return err
}

func (g GrpcClient) DeleteConfig(ctx context.Context, req *defangv1.Secrets) error {
	// _, err := g.client.DeleteSecrets(ctx, &connect.Request[v1.Secrets]{Msg: req}); TODO: implement this in the server
	var errs []error
	for _, name := range req.Names {
		_, err := g.client.PutSecret(ctx, &connect.Request[defangv1.SecretValue]{Msg: &defangv1.SecretValue{Name: name}})
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func (g GrpcClient) ListConfig(ctx context.Context) (*defangv1.Secrets, error) {
	return getMsg(g.client.ListSecrets(ctx, &connect.Request[emptypb.Empty]{}))
}

func (g GrpcClient) CreateUploadURL(ctx context.Context, req *defangv1.UploadURLRequest) (*defangv1.UploadURLResponse, error) {
	return getMsg(g.client.CreateUploadURL(ctx, &connect.Request[defangv1.UploadURLRequest]{Msg: req}))
}

func (g GrpcClient) WhoAmI(ctx context.Context) (*defangv1.WhoAmIResponse, error) {
	return getMsg(g.client.WhoAmI(ctx, &connect.Request[emptypb.Empty]{}))
}

func (g GrpcClient) DelegateSubdomainZone(ctx context.Context, req *defangv1.DelegateSubdomainZoneRequest) (*defangv1.DelegateSubdomainZoneResponse, error) {
	return getMsg(g.client.DelegateSubdomainZone(ctx, &connect.Request[defangv1.DelegateSubdomainZoneRequest]{Msg: req}))
}

func (g GrpcClient) DeleteSubdomainZone(ctx context.Context) error {
	_, err := getMsg(g.client.DeleteSubdomainZone(ctx, &connect.Request[emptypb.Empty]{}))
	return err
}

func (g GrpcClient) GetDelegateSubdomainZone(ctx context.Context) (*defangv1.DelegateSubdomainZoneResponse, error) {
	return getMsg(g.client.GetDelegateSubdomainZone(ctx, &connect.Request[emptypb.Empty]{}))
}

func (g *GrpcClient) Tail(ctx context.Context, req *defangv1.TailRequest) (ServerStream[defangv1.TailResponse], error) {
	return g.client.Tail(ctx, &connect.Request[defangv1.TailRequest]{Msg: req})
}

func (g *GrpcClient) BootstrapCommand(ctx context.Context, command string) (types.ETag, error) {
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

func (g *GrpcClient) CheckLoginAndToS(ctx context.Context) error {
	_, err := g.client.CheckToS(ctx, &connect.Request[emptypb.Empty]{})
	return err
}

func (g *GrpcClient) Destroy(ctx context.Context) (types.ETag, error) {
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

func (g *GrpcClient) TearDown(ctx context.Context) error {
	return errors.New("the teardown command is not valid for the Defang provider")
}

func (g *GrpcClient) BootstrapList(context.Context) ([]string, error) {
	return nil, errors.New("this command is not valid for the Defang provider")
}

func (g *GrpcClient) Restart(ctx context.Context, names ...string) (types.ETag, error) {
	// For now, we'll just get the service info and pass it back to Deploy as-is.
	services := make([]*defangv1.Service, 0, len(names))
	for _, name := range names {
		serviceInfo, err := g.Get(ctx, &defangv1.ServiceID{Name: name})
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

func (g GrpcClient) ServiceDNS(name string) string {
	whoami, _ := g.WhoAmI(context.TODO())
	return whoami.Tenant + "-" + name
}

func (g GrpcClient) LoadProjectName() (string, error) {
	return string(g.tenantID), nil
}
