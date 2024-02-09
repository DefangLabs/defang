package client

import (
	"context"

	v1 "github.com/defang-io/defang/src/protos/io/defang/v1"
)

type MockClient struct {
	Client
	UploadUrl string
}

var _ Client = (*MockClient)(nil)

func (m MockClient) CreateUploadURL(ctx context.Context, req *v1.UploadURLRequest) (*v1.UploadURLResponse, error) {
	return &v1.UploadURLResponse{Url: m.UploadUrl + req.Digest}, nil
}
