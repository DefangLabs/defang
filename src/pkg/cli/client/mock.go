package client

import (
	"context"

	pb "github.com/defang-io/defang/src/protos/io/defang/v1"
)

type MockClient struct {
	Client
	UploadUrl string
}

var _ Client = (*MockClient)(nil)

func (m MockClient) CreateUploadURL(ctx context.Context, req *pb.UploadURLRequest) (*pb.UploadURLResponse, error) {
	return &pb.UploadURLResponse{Url: m.UploadUrl + req.Digest}, nil
}
