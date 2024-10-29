package client

import (
	"context"
	"errors"
	"io"

	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	composeTypes "github.com/compose-spec/compose-go/v2/types"
)

type MockProvider struct {
	Provider
	UploadUrl    string
	Project      *composeTypes.Project
	ServerStream ServerStream[defangv1.TailResponse]
}

func (m MockProvider) CreateUploadURL(ctx context.Context, req *defangv1.UploadURLRequest) (*defangv1.UploadURLResponse, error) {
	return &defangv1.UploadURLResponse{Url: m.UploadUrl + req.Digest}, nil
}

func (m MockProvider) ListConfig(ctx context.Context) (*defangv1.Secrets, error) {
	return &defangv1.Secrets{Names: []string{"VAR1"}}, nil
}

func (m MockProvider) ServiceDNS(service string) string {
	return service
}

func (m MockProvider) LoadProject(ctx context.Context) (*composeTypes.Project, error) {
	return m.Project, nil
}

func (m MockProvider) LoadProjectName(ctx context.Context) (string, error) {
	return m.Project.Name, nil
}

func (m MockProvider) SetProjectName(projectName string) {
	m.Project.Name = projectName
}

func (m MockProvider) Follow(ctx context.Context, req *defangv1.TailRequest) (ServerStream[defangv1.TailResponse], error) {
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
