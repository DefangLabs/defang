package cli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/sirupsen/logrus"
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

func TestLoadCompose(t *testing.T) {
	DoVerbose = true
	term.SetDebug(true)

	t.Run("no project name defaults to parent directory name", func(t *testing.T) {
		loader := ComposeLoader{"../../tests/noprojname/compose.yaml"}
		p, err := loader.LoadCompose(context.Background())
		if err != nil {
			t.Fatalf("LoadCompose() failed: %v", err)
		}
		if p.Name != "noprojname" { // Use the parent directory name as project name
			t.Errorf("LoadCompose() failed: expected project name tenant-id, got %q", p.Name)
		}
	})

	t.Run("no project name defaults to fancy parent directory name", func(t *testing.T) {
		loader := ComposeLoader{"../../tests/Fancy-Proj_Dir/compose.yaml"}
		p, err := loader.LoadCompose(context.Background())
		if err != nil {
			t.Fatalf("LoadCompose() failed: %v", err)
		}
		if p.Name != "fancy-proj_dir" { // Use the parent directory name as project name
			t.Errorf("LoadCompose() failed: expected project name tenant-id, got %q", p.Name)
		}
	})

	t.Run("use project name in compose file", func(t *testing.T) {
		loader := ComposeLoader{"../../tests/testproj/compose.yaml"}
		p, err := loader.LoadCompose(context.Background())
		if err != nil {
			t.Fatalf("LoadCompose() failed: %v", err)
		}
		if p.Name != "tests" {
			t.Errorf("LoadCompose() failed: expected project name, got %q", p.Name)
		}
	})

	t.Run("COMPOSE_PROJECT_NAME env var should override project name", func(t *testing.T) {
		t.Setenv("COMPOSE_PROJECT_NAME", "overridename")
		loader := ComposeLoader{"../../tests/testproj/compose.yaml"}
		p, err := loader.LoadCompose(context.Background())
		if err != nil {
			t.Fatalf("LoadCompose() failed: %v", err)
		}
		if p.Name != "overridename" {
			t.Errorf("LoadCompose() failed: expected project name to be overwritten by env var, got %q", p.Name)
		}
	})

	t.Run("use project name should not be overriden by tenantID", func(t *testing.T) {
		loader := ComposeLoader{"../../tests/testproj/compose.yaml"}
		p, err := loader.LoadCompose(context.Background())
		if err != nil {
			t.Fatalf("LoadCompose() failed: %v", err)
		}
		if p.Name != "tests" {
			t.Errorf("LoadCompose() failed: expected project name tests, got %q", p.Name)
		}
	})

	t.Run("load starting from a sub directory", func(t *testing.T) {
		cwd, _ := os.Getwd()

		// setup
		setup := func() {
			os.MkdirAll("../../tests/alttestproj/subdir/subdir2", 0755)
			os.Chdir("../../tests/alttestproj/subdir/subdir2")
		}

		//teardown
		teardown := func() {
			os.Chdir(cwd)
			os.RemoveAll("../../tests/alttestproj/subdir")
		}

		setup()
		defer teardown()

		// execute test
		loader := ComposeLoader{}
		p, err := loader.LoadCompose(context.Background())
		if err != nil {
			t.Fatalf("LoadCompose() failed: %v", err)
		}
		if p.Name != "tests" {
			t.Errorf("LoadCompose() failed: expected project name, got %q", p.Name)
		}
	})

	t.Run("load alternative compose file", func(t *testing.T) {
		loader := ComposeLoader{"../../tests/alttestproj/altcomp.yaml"}
		p, err := loader.LoadCompose(context.Background())
		if err != nil {
			t.Fatalf("LoadCompose() failed: %v", err)
		}
		if p.Name != "tests" {
			t.Errorf("LoadCompose() failed: expected project name, got %q", p.Name)
		}
	})
}

func TestConvertPort(t *testing.T) {
	tests := []struct {
		name     string
		input    types.ServicePortConfig
		expected *defangv1.Port
		wantErr  string
	}{
		{
			name:    "No target port xfail",
			input:   types.ServicePortConfig{},
			wantErr: "port 'target' must be an integer between 1 and 32767",
		},
		{
			name:     "Undefined mode and protocol, target only",
			input:    types.ServicePortConfig{Target: 1234},
			expected: &defangv1.Port{Target: 1234, Mode: defangv1.Mode_INGRESS},
		},
		{
			name:     "Undefined mode and protocol, published equals target",
			input:    types.ServicePortConfig{Target: 1234, Published: "1234"},
			expected: &defangv1.Port{Target: 1234, Mode: defangv1.Mode_INGRESS},
		},
		{
			name:     "Undefined mode, udp protocol, target only",
			input:    types.ServicePortConfig{Target: 1234, Protocol: "udp"},
			expected: &defangv1.Port{Target: 1234, Mode: defangv1.Mode_HOST, Protocol: defangv1.Protocol_UDP}, // backwards compatibility
		},
		{
			name:     "Undefined mode and published range xfail",
			input:    types.ServicePortConfig{Target: 1234, Published: "1511-2222"},
			expected: &defangv1.Port{Target: 1234, Mode: defangv1.Mode_INGRESS},
		},
		{
			name:     "Undefined mode and target in published range xfail",
			input:    types.ServicePortConfig{Target: 1234, Published: "1111-2222"},
			expected: &defangv1.Port{Target: 1234, Mode: defangv1.Mode_INGRESS},
		},
		{
			name:     "Undefined mode and published not equals target; common for local development",
			input:    types.ServicePortConfig{Target: 1234, Published: "12345"},
			expected: &defangv1.Port{Target: 1234, Mode: defangv1.Mode_INGRESS},
		},
		{
			name:     "Host mode and undefined protocol, target only",
			input:    types.ServicePortConfig{Mode: "host", Target: 1234},
			expected: &defangv1.Port{Target: 1234, Mode: defangv1.Mode_HOST},
		},
		{
			name:     "Host mode and udp protocol, target only",
			input:    types.ServicePortConfig{Mode: "host", Target: 1234, Protocol: "udp"},
			expected: &defangv1.Port{Target: 1234, Mode: defangv1.Mode_HOST, Protocol: defangv1.Protocol_UDP},
		},
		{
			name:     "Host mode and protocol, published equals target",
			input:    types.ServicePortConfig{Mode: "host", Target: 1234, Published: "1234"},
			expected: &defangv1.Port{Target: 1234, Mode: defangv1.Mode_HOST},
		},
		{
			name:    "Host mode and protocol, published range xfail",
			input:   types.ServicePortConfig{Mode: "host", Target: 1234, Published: "1511-2222"},
			wantErr: "port 'published' range must include 'target': 1511-2222",
		},
		{
			name:    "Host mode and protocol, published range xfail",
			input:   types.ServicePortConfig{Mode: "host", Target: 1234, Published: "22222"},
			wantErr: "port 'published' must be empty or equal to 'target': 22222",
		},
		{
			name:     "Host mode and protocol, target in published range",
			input:    types.ServicePortConfig{Mode: "host", Target: 1234, Published: "1111-2222"},
			expected: &defangv1.Port{Target: 1234, Mode: defangv1.Mode_HOST},
		},
		{
			name:     "(Implied) ingress mode, defined protocol, only target", // - 1234
			input:    types.ServicePortConfig{Mode: "ingress", Protocol: "tcp", Target: 1234},
			expected: &defangv1.Port{Target: 1234, Mode: defangv1.Mode_INGRESS, Protocol: defangv1.Protocol_HTTP},
		},
		{
			name:     "(Implied) ingress mode, udp protocol, only target", // - 1234/udp
			input:    types.ServicePortConfig{Mode: "ingress", Protocol: "udp", Target: 1234},
			expected: &defangv1.Port{Target: 1234, Mode: defangv1.Mode_HOST, Protocol: defangv1.Protocol_UDP}, // backwards compatibility
		},
		{
			name:     "(Implied) ingress mode, defined protocol, published equals target", // - 1234:1234
			input:    types.ServicePortConfig{Mode: "ingress", Protocol: "tcp", Published: "1234", Target: 1234},
			expected: &defangv1.Port{Target: 1234, Mode: defangv1.Mode_INGRESS, Protocol: defangv1.Protocol_HTTP},
		},
		{
			name:     "(Implied) ingress mode, udp protocol, published equals target", // - 1234:1234/udp
			input:    types.ServicePortConfig{Mode: "ingress", Protocol: "udp", Published: "1234", Target: 1234},
			expected: &defangv1.Port{Target: 1234, Mode: defangv1.Mode_HOST, Protocol: defangv1.Protocol_UDP}, // backwards compatibility
		},
		{
			name:    "Localhost IP, unsupported mode and protocol xfail",
			input:   types.ServicePortConfig{Mode: "ingress", HostIP: "127.0.0.1", Protocol: "tcp", Published: "1234", Target: 1234},
			wantErr: "port 'host_ip' is not supported",
		},
		{
			name:     "Ingress mode without host IP, single target, published range xfail", // - 1511-2223:1234
			input:    types.ServicePortConfig{Mode: "ingress", Protocol: "tcp", Target: 1234, Published: "1511-2223"},
			expected: &defangv1.Port{Target: 1234, Mode: defangv1.Mode_INGRESS, Protocol: defangv1.Protocol_HTTP},
		},
		{
			name:     "Ingress mode without host IP, single target, target in published range", // - 1111-2223:1234
			input:    types.ServicePortConfig{Mode: "ingress", Protocol: "tcp", Target: 1234, Published: "1111-2223"},
			expected: &defangv1.Port{Target: 1234, Mode: defangv1.Mode_INGRESS, Protocol: defangv1.Protocol_HTTP},
		},
		{
			name:     "Ingress mode without host IP, published not equals target; common for local development", // - 12345:1234
			input:    types.ServicePortConfig{Mode: "ingress", Protocol: "tcp", Target: 1234, Published: "12345"},
			expected: &defangv1.Port{Target: 1234, Mode: defangv1.Mode_INGRESS, Protocol: defangv1.Protocol_HTTP},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePort(tt.input)
			if err != nil {
				if tt.wantErr == "" {
					t.Errorf("convertPort() unexpected error: %v", err)
				} else if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("convertPort() error = %v, wantErr %v", err, tt.wantErr)
				}
				return
			}
			if tt.wantErr != "" {
				t.Errorf("convertPort() expected error: %v", tt.wantErr)
			}
			got := convertPort(tt.input)
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
		url, err := uploadTarball(context.Background(), client.MockClient{UploadUrl: server.URL + path}, &bytes.Buffer{}, digest)
		if err != nil {
			t.Fatalf("uploadTarball() failed: %v", err)
		}
		const expectedPath = path + digest
		if url != server.URL+expectedPath {
			t.Errorf("Expected %v, got %v", server.URL+expectedPath, url)
		}
	})

	t.Run("force upload without digest", func(t *testing.T) {
		url, err := uploadTarball(context.Background(), client.MockClient{UploadUrl: server.URL + path}, &bytes.Buffer{}, "")
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
		buffer, err := createTarball(context.Background(), "../../tests/testproj", "")
		if err != nil {
			t.Fatalf("createTarballReader() failed: %v", err)
		}

		g, err := gzip.NewReader(buffer)
		if err != nil {
			t.Fatalf("gzip.NewReader() failed: %v", err)
		}
		defer g.Close()

		expected := []string{".dockerignore", "Dockerfile", "fileName.env"}
		var actual []string
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
			actual = append(actual, h.Name)
		}
		if !reflect.DeepEqual(actual, expected) {
			t.Errorf("Expected files: %v, got %v", expected, actual)
		}
	})

	t.Run("Missing Dockerfile", func(t *testing.T) {
		_, err := createTarball(context.Background(), "../../tests", "Dockerfile.missing")
		if err == nil {
			t.Fatal("createTarballReader() should have failed")
		}
	})

	t.Run("Missing Context", func(t *testing.T) {
		_, err := createTarball(context.Background(), "asdfqwer", "")
		if err == nil {
			t.Fatal("createTarballReader() should have failed")
		}
	})
}

func TestProjectValidationServiceName(t *testing.T) {
	loader := ComposeLoader{"../../tests/testproj/compose.yaml"}
	p, err := loader.LoadCompose(context.Background())
	if err != nil {
		t.Fatalf("LoadCompose() failed: %v", err)
	}

	if err := validateProject(p); err != nil {
		t.Fatalf("Project validation failed: %v", err)
	}

	svc := p.Services["dfnx"]
	longName := "aVeryLongServiceNameThatIsDefinitelyTooLongThatWillCauseAnError"
	svc.Name = longName
	p.Services[longName] = svc

	if err := validateProject(p); err == nil {
		t.Fatalf("Long project name should be an error")
	}

}

func TestProjectValidationNetworks(t *testing.T) {
	var warnings bytes.Buffer
	logrus.SetOutput(&warnings)

	loader := ComposeLoader{"../../tests/testproj/compose.yaml"}
	p, err := loader.LoadCompose(context.Background())
	if err != nil {
		t.Fatalf("LoadCompose() failed: %v", err)
	}

	dfnx := p.Services["dfnx"]
	dfnx.Networks = map[string]*types.ServiceNetworkConfig{"invalid-network-name": nil}
	p.Services["dfnx"] = dfnx
	if err := validateProject(p); err != nil {
		t.Errorf("Invalid network name should not be an error: %v", err)
	}
	if !bytes.Contains(warnings.Bytes(), []byte(`network \"invalid-network-name\" is not defined`)) {
		t.Errorf("Invalid network name should trigger a warning: %v", warnings.String())
	}

	warnings.Reset()
	dfnx.Networks = map[string]*types.ServiceNetworkConfig{"public": nil}
	p.Services["dfnx"] = dfnx
	if err := validateProject(p); err != nil {
		t.Errorf("public network name should not be an error: %v", err)
	}
	if !bytes.Contains(warnings.Bytes(), []byte(`network \"public\" is not defined`)) {
		t.Errorf("missing public network in global networks section should trigger a warning: %v", warnings.String())
	}

	warnings.Reset()
	p.Networks["public"] = types.NetworkConfig{}
	if err := validateProject(p); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if bytes.Contains(warnings.Bytes(), []byte(`network \"public\" is not defined`)) {
		t.Errorf("When public network is defined globally should not trigger a warning when public network is used")
	}
}

func TestComposeGoNoDoubleWarningLog(t *testing.T) {
	var warnings bytes.Buffer
	logrus.SetOutput(&warnings)

	loader := ComposeLoader{"../../tests/compose-go-warn/compose.yaml"}
	_, err := loader.LoadCompose(context.Background())
	if err != nil {
		t.Fatalf("LoadCompose() failed: %v", err)
	}

	if bytes.Count(warnings.Bytes(), []byte(`\"yes\" for boolean is not supported by YAML 1.2`)) != 1 {
		t.Errorf("Warning for using 'yes' for boolean from compose-go should appear exactly once")
	}
}

func TestProjectValidationNoDeploy(t *testing.T) {
	loader := ComposeLoader{"../../tests/testproj/compose.yaml"}
	p, err := loader.LoadCompose(context.Background())
	if err != nil {
		t.Fatalf("LoadCompose() failed: %v", err)
	}

	dfnx := p.Services["dfnx"]
	dfnx.Deploy = nil
	p.Services["dfnx"] = dfnx
	if err := validateProject(p); err != nil {
		t.Errorf("No deploy section should not be an error: %v", err)
	}
}

func TestIsStatefulImage(t *testing.T) {
	tests := []struct {
		name     string
		image    string
		expected bool
	}{
		{
			name:     "Stateful image",
			image:    "redis",
			expected: true,
		},
		{
			name:     "Stateful image with repo",
			image:    "library/redis",
			expected: true,
		},
		{
			name:     "Stateful image with tag",
			image:    "redis:6.0",
			expected: true,
		},
		{
			name:     "Stateful image with registry",
			image:    "docker.io/redis",
			expected: true,
		},
		{
			name:     "Stateless image",
			image:    "alpine:latest",
			expected: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isStatefulImage(tt.image); got != tt.expected {
				t.Errorf("isStatefulImage() = %v, want %v", got, tt.expected)
			}
		})
	}
}
