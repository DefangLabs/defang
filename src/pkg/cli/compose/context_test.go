package compose

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/moby/patternmatcher/ignorefile"
)

func Test_parseContextLimit(t *testing.T) {
	t.Run("valid limit", func(t *testing.T) {
		if got := parseContextLimit("1MiB", 0); got != MiB {
			t.Errorf("Expected %v, got %v", MiB, got)
		}
	})

	t.Run("invalid limit", func(t *testing.T) {
		if got := parseContextLimit("invalid", 42); got != 42 {
			t.Errorf("Expected 42, got %v", got)
		}
	})

	t.Run("empty limit", func(t *testing.T) {
		if got := parseContextLimit("", 42); got != 42 {
			t.Errorf("Expected 42, got %v", got)
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
		url, err := uploadTarball(context.Background(), client.MockProvider{UploadUrl: server.URL + path}, "testproj", &bytes.Buffer{}, digest)
		if err != nil {
			t.Fatalf("uploadTarball() failed: %v", err)
		}
		const expectedPath = path + digest
		if url != server.URL+expectedPath {
			t.Errorf("Expected %v, got %v", server.URL+expectedPath, url)
		}
	})

	t.Run("force upload without digest", func(t *testing.T) {
		url, err := uploadTarball(context.Background(), client.MockProvider{UploadUrl: server.URL + path}, "testproj", &bytes.Buffer{}, "")
		if err != nil {
			t.Fatalf("uploadTarball() failed: %v", err)
		}
		if url != server.URL+path {
			t.Errorf("Expected %v, got %v", server.URL+path, url)
		}
	})
}

func TestWalkContextFolder(t *testing.T) {
	t.Run("Default Dockerfile", func(t *testing.T) {
		var files []string
		err := WalkContextFolder("../../../testdata/testproj", "", func(path string, de os.DirEntry, slashPath string) error {
			if strings.Contains(slashPath, "testproj") {
				t.Errorf("Path is not relative: %v", slashPath)
			}
			files = append(files, slashPath)
			return nil
		})
		if err != nil {
			t.Fatalf("WalkContextFolder() failed: %v", err)
		}

		expected := []string{".dockerignore", ".env", "Dockerfile", "fileName.env"}
		if !reflect.DeepEqual(files, expected) {
			t.Errorf("Expected files: %v, got %v", expected, files)
		}
	})

	t.Run("Missing Dockerfile", func(t *testing.T) {
		err := WalkContextFolder("../../testdata", "Dockerfile.missing", func(string, os.DirEntry, string) error { return nil })
		if err == nil {
			t.Fatal("WalkContextFolder() should have failed")
		}
	})

	t.Run("Missing Context", func(t *testing.T) {
		err := WalkContextFolder("asdfqwer", "", func(string, os.DirEntry, string) error { return nil })
		if err == nil {
			t.Fatal("WalkContextFolder() should have failed")
		}
	})

	t.Run("Default .dockerignore", func(t *testing.T) {
		var files []string
		err := WalkContextFolder("../../../testdata/alttestproj", "", func(path string, de os.DirEntry, slashPath string) error {
			if strings.Contains(slashPath, "alttestproj") {
				t.Errorf("Path is not relative: %v", slashPath)
			}
			files = append(files, slashPath)
			return nil
		})
		if err != nil {
			t.Fatalf("WalkContextFolder() failed: %v", err)
		}

		expected := []string{".dockerignore", "Dockerfile", "altcomp.yaml", "compose.yaml.fixup", "compose.yaml.golden", "compose.yaml.warnings"}
		if !reflect.DeepEqual(files, expected) {
			t.Errorf("Expected files: %v, got %v", expected, files)
		}
	})
}

func TestCreateTarballReader(t *testing.T) {
	t.Run("Default Dockerfile", func(t *testing.T) {
		buffer, err := createTarball(context.Background(), "../../../testdata/testproj", "")
		if err != nil {
			t.Fatalf("createTarballReader() failed: %v", err)
		}

		g, err := gzip.NewReader(buffer)
		if err != nil {
			t.Fatalf("gzip.NewReader() failed: %v", err)
		}
		defer g.Close()

		expected := []string{".dockerignore", ".env", "Dockerfile", "fileName.env"}
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
		_, err := createTarball(context.Background(), "../../testdata", "Dockerfile.missing")
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

func TestGetDockerIgnorePatterns(t *testing.T) {
	tests := []struct {
		name              string
		dockerfile        string
		ignoreFileName    string
		ignoreFileContent string
		expectedFileName  string
	}{
		{
			name:              "dockerfile-specific and ignore file exists",
			dockerfile:        "DefangDockerfile",
			ignoreFileName:    "DefangDockerfile.dockerignore",
			ignoreFileContent: "**/node_modules\n**/build",
			expectedFileName:  "DefangDockerfile.dockerignore",
		},
		{
			name:              "Regular dockerfile and dockerignore exists",
			dockerfile:        "Dockerfile",
			ignoreFileName:    ".dockerignore",
			ignoreFileContent: "**/dist\n**/.env",
			expectedFileName:  ".dockerignore",
		},
		{
			name:              "No .dockerignore, but dockerfile exists",
			dockerfile:        "Dockerfile",
			ignoreFileName:    "",
			ignoreFileContent: defaultDockerIgnore,
			expectedFileName:  ".dockerignore",
		},
		{
			name:              "No dockerfile, but dockerignore exists",
			dockerfile:        "",
			ignoreFileName:    ".dockerignore",
			ignoreFileContent: defaultDockerIgnore,
			expectedFileName:  ".dockerignore",
		},
		{
			name:              "No dockerfile, and no dockerignore exists",
			dockerfile:        "",
			ignoreFileName:    "",
			ignoreFileContent: defaultDockerIgnore,
			expectedFileName:  ".dockerignore",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new temporary directory for this test case
			tempDir := t.TempDir()

			// Create specified ignore file if the name is not empty
			if tt.ignoreFileName != "" {
				ignoreFilePath := filepath.Join(tempDir, tt.ignoreFileName)
				err := os.WriteFile(ignoreFilePath, []byte(tt.ignoreFileContent), 0644)
				if err != nil {
					t.Fatalf("Failed to create ignore file: %v", err)
				}
			}

			// Call the function under test
			patterns, fileName, err := getDockerIgnorePatterns(tempDir, tt.dockerfile)
			if err != nil {
				t.Fatalf("Failed to get ignore file pattern: %v", err)
			}

			// Verify the returned file name
			if fileName != tt.expectedFileName {
				t.Errorf("Expected file name %s, but got %s", tt.expectedFileName, fileName)
			}

			// Verify the content of the patterns
			if tt.ignoreFileContent != "" {
				// Make expected patterns to test against
				expectedPatterns, err := ignorefile.ReadAll(strings.NewReader(tt.ignoreFileContent))
				if err != nil {
					t.Fatalf("Failed to retrieve expected patterns: %v", err)
				}
				if !reflect.DeepEqual(patterns, expectedPatterns) {
					t.Errorf("Expected patterns %v, but got %v", expectedPatterns, patterns)
				}
			}
		})
	}
}
