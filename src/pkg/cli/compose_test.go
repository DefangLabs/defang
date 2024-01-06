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

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/defang-io/defang/src/pkg/cli/client"
	pb "github.com/defang-io/defang/src/protos/io/defang/v1"
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
		p, err := loadDockerCompose("../../tests/docker-compose.yml", "blah")
		if err != nil {
			t.Fatalf("loadDockerCompose() failed: %v", err)
		}
		if p.Name != "blah" {
			t.Errorf("loadDockerCompose() failed: expected project name, got %q", p.Name)
		}
	})

	t.Run("fancy project name", func(t *testing.T) {
		p, err := loadDockerCompose("../../tests/docker-compose.yml", "Valid-Username")
		if err != nil {
			t.Fatalf("loadDockerCompose() failed: %v", err)
		}
		if p.Name != "valid-username" {
			t.Errorf("loadDockerCompose() failed: expected project name, got %q", p.Name)
		}
	})
}

func TestConvertPort(t *testing.T) {
	tests := []struct {
		name     string
		input    types.ServicePortConfig
		expected *pb.Port
		wantErr  string
	}{
		{
			name:    "No target port xfail",
			input:   types.ServicePortConfig{},
			wantErr: "port target must be an integer between 1 and 32767",
		},
		{
			name:     "Undefined mode and protocol, target only",
			input:    types.ServicePortConfig{Target: 1234},
			expected: &pb.Port{Target: 1234, Mode: pb.Mode_HOST},
		},
		{
			name:    "Published range xfail",
			input:   types.ServicePortConfig{Target: 1234, Published: "1111-2222"},
			wantErr: "port published must be empty or equal to target: 1111-2222",
		},
		{
			name:     "Implied ingress mode, defined protocol, published equals target",
			input:    types.ServicePortConfig{Mode: "ingress", Protocol: "tcp", Published: "1234", Target: 1234},
			expected: &pb.Port{Target: 1234, Mode: pb.Mode_HOST, Protocol: pb.Protocol_TCP},
		},
		{
			name:     "Implied ingress mode, udp protocol, published equals target",
			input:    types.ServicePortConfig{Mode: "ingress", Protocol: "udp", Published: "1234", Target: 1234},
			expected: &pb.Port{Target: 1234, Mode: pb.Mode_HOST, Protocol: pb.Protocol_UDP},
		},
		{
			name:    "Localhost IP, unsupported mode and protocol xfail",
			input:   types.ServicePortConfig{Mode: "ingress", HostIP: "127.0.0.1", Protocol: "tcp", Published: "1234", Target: 1234},
			wantErr: "host_ip is not supported",
		},
		{
			name:     "Ingress mode without host IP, single target",
			input:    types.ServicePortConfig{Mode: "ingress", Protocol: "tcp", Target: 1234},
			expected: &pb.Port{Target: 1234, Mode: pb.Mode_INGRESS, Protocol: pb.Protocol_HTTP},
		},
		{
			name:    "Ingress mode without host IP, single target, published range xfail",
			input:   types.ServicePortConfig{Mode: "ingress", Protocol: "tcp", Target: 1234, Published: "1111-2223"},
			wantErr: "port published must be empty or equal to target: 1111-2223",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := convertPort(tt.input)
			if err != nil {
				if tt.wantErr == "" {
					t.Errorf("convertPort() unexpected error: %v", err)
				} else if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("convertPort() error = %v, wantErr %v", err, tt.wantErr)
				}
				return
			}
			if got.String() != tt.expected.String() {
				t.Errorf("convertPort() got %v, want %v", got, tt.expected.String())
			}
		})
	}
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
		url, err := uploadTarball(context.TODO(), client.MockClient{server.URL + path}, &bytes.Buffer{}, digest)
		if err != nil {
			t.Fatalf("uploadTarball() failed: %v", err)
		}
		const expectedPath = path + digest
		if url != server.URL+expectedPath {
			t.Errorf("Expected %v, got %v", server.URL+expectedPath, url)
		}
	})

	t.Run("force upload without digest", func(t *testing.T) {
		url, err := uploadTarball(context.TODO(), client.MockClient{server.URL + path}, &bytes.Buffer{}, "")
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
		buffer, err := createTarball(context.TODO(), "../../tests", "")
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
		_, err := createTarball(context.TODO(), "../../tests", "Dockerfile.missing")
		if err == nil {
			t.Fatal("createTarballReader() should have failed")
		}
	})

	t.Run("Missing Context", func(t *testing.T) {
		_, err := createTarball(context.TODO(), "asdfqwer", "")
		if err == nil {
			t.Fatal("createTarballReader() should have failed")
		}
	})
}
