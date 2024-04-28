package client

import (
	"context"

	defangv1 "github.com/defang-io/defang/src/protos/io/defang/v1"
)

type MockClient struct {
	Client
	UploadUrl string
}

var _ Client = (*MockClient)(nil)

func (m MockClient) CreateUploadURL(ctx context.Context, req *defangv1.UploadURLRequest) (*defangv1.UploadURLResponse, error) {
	return &defangv1.UploadURLResponse{Url: m.UploadUrl + req.Digest}, nil
}

func (m MockClient) ServiceDNS(service string) string {
	return service
}
