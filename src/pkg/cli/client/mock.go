package client

import (
	"context"

	pb "github.com/defang-io/defang/src/protos/io/defang/v1"
)

type MockClient struct {
	UploadUrl string
}

var _ Client = (*MockClient)(nil)

func (m MockClient) CreateUploadURL(ctx context.Context, req *pb.UploadURLRequest) (*pb.UploadURLResponse, error) {
	return &pb.UploadURLResponse{Url: m.UploadUrl + req.Digest}, nil
}
func (MockClient) GetStatus(context.Context) (*pb.Status, error) {
	panic("no impl: GetStatus")
}
func (MockClient) GetVersion(context.Context) (*pb.Version, error) {
	panic("no impl: GetVersion")
}
func (MockClient) Tail(context.Context, *pb.TailRequest) (ServerStream[pb.TailResponse], error) {
	panic("no impl: Tail")
}
func (MockClient) Update(context.Context, *pb.Service) (*pb.ServiceInfo, error) {
	panic("no impl: Update")
}
func (MockClient) Deploy(context.Context, *pb.DeployRequest) (*pb.DeployResponse, error) {
	panic("no impl: Deploy")
}
func (MockClient) Get(context.Context, *pb.ServiceID) (*pb.ServiceInfo, error) {
	panic("no impl: Get")
}
func (MockClient) Delete(context.Context, *pb.DeleteRequest) (*pb.DeleteResponse, error) {
	panic("no impl: Delete")
}
func (MockClient) Publish(context.Context, *pb.PublishRequest) error {
	panic("no impl: Publish")
}
func (MockClient) Subscribe(context.Context, *pb.SubscribeRequest) (*pb.SubscribeResponse, error) {
	panic("no impl: Subscribe")
}
func (MockClient) GetServices(context.Context) (*pb.ListServicesResponse, error) {
	panic("no impl: GetServices")
}
func (MockClient) Token(context.Context, *pb.TokenRequest) (*pb.TokenResponse, error) {
	panic("no impl: Token")
}
func (MockClient) PutSecret(context.Context, *pb.SecretValue) error {
	panic("no impl: PutSecret")
}
func (MockClient) ListSecrets(context.Context) (*pb.Secrets, error) {
	panic("no impl: ListSecrets")
}
func (MockClient) GenerateFiles(context.Context, *pb.GenerateFilesRequest) (*pb.GenerateFilesResponse, error) {
	panic("no impl: GenerateFiles")
}
func (MockClient) RevokeToken(context.Context) error {
	panic("no impl: RevokeToken")
}
