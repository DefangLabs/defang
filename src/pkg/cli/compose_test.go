package cli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bufbuild/connect-go"
	pb "github.com/defang-io/defang/src/protos/io/defang/v1"
	"google.golang.org/protobuf/types/known/emptypb"
)

func TestNormalizeServiceName(t *testing.T) {
	testCases := []struct {
		name     string
		expected string
	}{
		{name: "normal", expected: "normal"},
		{name: "camelCase", expected: "camelcase"},
		{name: "PascalCase", expected: "pascalcase"},
		{name: "hyphen-ok", expected: "hyphen-ok"},
		{name: "snake_case", expected: "snake-case"},
		{name: "$ymb0ls", expected: "-ymb0ls"},
		{name: "consecutive--hyphens", expected: "consecutive-hyphens"},
		{name: "hyphen-$ymbol", expected: "hyphen-ymbol"},
		{name: "_blah", expected: "-blah"},
	}
	for _, tC := range testCases {
		t.Run(tC.name, func(t *testing.T) {
			actual := NormalizeServiceName(tC.name)
			if actual != tC.expected {
				t.Errorf("NormalizeServiceName() failed: expected %v, got %v", tC.expected, actual)
			}
		})
	}
}

func TestLoadDockerCompose(t *testing.T) {
	DoVerbose = true

	t.Run("no project name", func(t *testing.T) {
		_, err := loadDockerCompose("../../tests/docker-compose.yml", "")
		if err != nil {
			t.Fatalf("loadDockerCompose() failed: %v", err)
		}
	})

	t.Run("override project name", func(t *testing.T) {
		_, err := loadDockerCompose("../../tests/docker-compose.yml", "blah")
		if err != nil {
			t.Fatalf("loadDockerCompose() failed: %v", err)
		}
	})

	t.Run("fancy project name", func(t *testing.T) {
		_, err := loadDockerCompose("../../tests/docker-compose.yml", "Valid-Username")
		if err != nil {
			t.Fatalf("loadDockerCompose() failed: %v", err)
		}
	})
}

func TestUploadTarball(t *testing.T) {
	const path = "/upload/x/"
	const digest = "sha256-47DEQpj8HBSa+/TImW+5JCeuQeRkm5NMpJWZG3hSuFU="

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PUT" {
			t.Errorf("Expected PUT request, got %v", r.Method)
		}
		if !strings.HasPrefix(r.URL.Path, path) {
			t.Errorf("Expected prefix %v, got %v", path, r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/gzip" {
			t.Errorf("Expected Content-Type: application/gzip, got %v", r.Header.Get("Content-Type"))
		}
		w.WriteHeader(200)
	}))
	defer server.Close()

	t.Run("upload with digest", func(t *testing.T) {
		url, err := uploadTarball(context.TODO(), mockGrpcClient{server.URL + path}, &bytes.Buffer{}, digest)
		if err != nil {
			t.Fatalf("uploadTarball() failed: %v", err)
		}
		const expectedPath = path + digest
		if url != server.URL+expectedPath {
			t.Errorf("Expected %v, got %v", server.URL+expectedPath, url)
		}
	})

	t.Run("force upload without digest", func(t *testing.T) {
		url, err := uploadTarball(context.TODO(), mockGrpcClient{server.URL + path}, &bytes.Buffer{}, "")
		if err != nil {
			t.Fatalf("uploadTarball() failed: %v", err)
		}
		if url != server.URL+path {
			t.Errorf("Expected %v, got %v", server.URL+path, url)
		}
	})
}

func TestCreateTarballReader(t *testing.T) {
	t.Run("Default Dockerfile", func(t *testing.T) {
		buffer, err := createTarballReader(context.TODO(), "../../tests", "")
		if err != nil {
			t.Fatalf("createTarballReader() failed: %v", err)
		}

		g, err := gzip.NewReader(buffer)
		if err != nil {
			t.Fatalf("gzip.NewReader() failed: %v", err)
		}
		defer g.Close()

		var foundDockerfile bool
		ar := tar.NewReader(g)
		for {
			h, err := ar.Next()
			if err != nil {
				if err == io.EOF {
					break
				}
				t.Fatal(err)
			}
			// Ensure the paths are relative
			if h.Name[0] == '/' {
				t.Errorf("Path is not relative: %v", h.Name)
			}
			if _, err := ar.Read(make([]byte, h.Size)); err != io.EOF {
				t.Log(err)
			}
			if h.Name == "Dockerfile" {
				foundDockerfile = true
			}
		}
		if !foundDockerfile {
			t.Error("Dockerfile not found in tarball")
		}
	})

	t.Run("Missing Dockerfile", func(t *testing.T) {
		_, err := createTarballReader(context.TODO(), "../../tests", "Dockerfile.missing")
		if err == nil {
			t.Fatal("createTarballReader() should have failed")
		}
	})

	t.Run("Missing Context", func(t *testing.T) {
		_, err := createTarballReader(context.TODO(), "asdfqwer", "")
		if err == nil {
			t.Fatal("createTarballReader() should have failed")
		}
	})
}

type mockGrpcClient struct {
	url string
}

func (m mockGrpcClient) CreateUploadURL(ctx context.Context, req *connect.Request[pb.UploadURLRequest]) (*connect.Response[pb.UploadURLResponse], error) {
	return connect.NewResponse(&pb.UploadURLResponse{Url: m.url + req.Msg.Digest}), nil
}
func (mockGrpcClient) GetStatus(context.Context, *connect.Request[emptypb.Empty]) (*connect.Response[pb.Status], error) {
	panic("no impl")
}
func (mockGrpcClient) GetVersion(context.Context, *connect.Request[emptypb.Empty]) (*connect.Response[pb.Version], error) {
	panic("no impl")
}
func (mockGrpcClient) Tail(context.Context, *connect.Request[pb.TailRequest]) (*connect.ServerStreamForClient[pb.TailResponse], error) {
	panic("no impl")
}
func (mockGrpcClient) Update(context.Context, *connect.Request[pb.Service]) (*connect.Response[pb.ServiceInfo], error) {
	panic("no impl")
}
func (mockGrpcClient) Get(context.Context, *connect.Request[pb.ServiceID]) (*connect.Response[pb.ServiceInfo], error) {
	panic("no impl")
}
func (mockGrpcClient) Delete(context.Context, *connect.Request[pb.DeleteRequest]) (*connect.Response[pb.DeleteResponse], error) {
	panic("no impl")
}
func (mockGrpcClient) Publish(context.Context, *connect.Request[pb.PublishRequest]) (*connect.Response[emptypb.Empty], error) {
	panic("no impl")
}
func (mockGrpcClient) Subscribe(context.Context, *connect.Request[pb.SubscribeRequest]) (*connect.ServerStreamForClient[pb.SubscribeResponse], error) {
	panic("no impl")
}
func (mockGrpcClient) GetServices(context.Context, *connect.Request[emptypb.Empty]) (*connect.Response[pb.ListServicesResponse], error) {
	panic("no impl")
}
func (mockGrpcClient) Token(context.Context, *connect.Request[pb.TokenRequest]) (*connect.Response[pb.TokenResponse], error) {
	panic("no impl")
}
func (mockGrpcClient) PutSecret(context.Context, *connect.Request[pb.SecretValue]) (*connect.Response[emptypb.Empty], error) {
	panic("no impl")
}
func (mockGrpcClient) ListSecrets(context.Context, *connect.Request[emptypb.Empty]) (*connect.Response[pb.Secrets], error) {
	panic("no impl")
}
func (mockGrpcClient) GenerateFiles(context.Context, *connect.Request[pb.GenerateFilesRequest]) (*connect.Response[pb.GenerateFilesResponse], error) {
	panic("no impl")
}
func (mockGrpcClient) RevokeToken(context.Context, *connect.Request[emptypb.Empty]) (*connect.Response[emptypb.Empty], error) {
	panic("no impl")
}
