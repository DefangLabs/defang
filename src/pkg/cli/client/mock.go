package client

import (
	"context"
	"errors"
	"io"

	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	compose "github.com/compose-spec/compose-go/v2/types"
)

type MockClient struct {
	Client
	UploadUrl    string
	Project      *compose.Project
	ServerStream ServerStream[defangv1.TailResponse]
}

func (m MockClient) CreateUploadURL(ctx context.Context, req *defangv1.UploadURLRequest) (*defangv1.UploadURLResponse, error) {
	return &defangv1.UploadURLResponse{Url: m.UploadUrl + req.Digest}, nil
}

func (m MockClient) ListConfigNames(ctx context.Context) ([]string, error) {
	configs, err := m.ListConfig(ctx)
	if err != nil {
		return nil, err
	}

	return configs.Names, nil
}

func (m MockClient) ListConfig(ctx context.Context) (*defangv1.Secrets, error) {
	return &defangv1.Secrets{Names: []string{"VAR1"}}, nil
}

func (m MockClient) ServiceDNS(service string) string {
	return service
}

func (m MockClient) LoadProject(ctx context.Context) (*compose.Project, error) {
	return m.Project, nil
}

func (m MockClient) LoadProjectName(ctx context.Context) (string, error) {
	return m.Project.Name, nil
}

func (m MockClient) SetProjectName(projectName string) {
	m.Project.Name = projectName
}

func (m MockClient) Follow(ctx context.Context, req *defangv1.TailRequest) (ServerStream[defangv1.TailResponse], error) {
	if m.ServerStream != nil {
		return m.ServerStream, nil
	}
	return nil, errors.New("no server stream provided")
}

type MockServerStream struct {
	Resps []*defangv1.TailResponse
	Errs  []error
}

func (m *MockServerStream) Close() error {
	return nil
}

func (m *MockServerStream) Receive() bool {
	if len(m.Resps) == 0 {
		return false
	}
	return true
}

func (m *MockServerStream) Msg() *defangv1.TailResponse {
	if len(m.Resps) == 0 {
		return nil
	}
	resp := m.Resps[0]
	m.Resps = m.Resps[1:]
	return resp
}

func (m *MockServerStream) Err() error {
	if len(m.Resps) == 0 && len(m.Errs) == 0 {
		return io.EOF // End of test
	}
	if len(m.Errs) == 0 {
		return errors.New("unexpected call to Err() for the test")
	}
	err := m.Errs[0]
	m.Errs = m.Errs[1:]
	return err
}
