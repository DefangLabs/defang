package compose

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/compose-spec/compose-go/v2/types"
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

func TestUploadArchive(t *testing.T) {
	const testproj = "testproj"
	const path = "/upload/x/"
	const digest = "sha256-47DEQpj8HBSa+/TImW+5JCeuQeRkm5NMpJWZG3hSuFU="

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PUT" {
			t.Errorf("Expected PUT request, got %v", r.Method)
		}
		if !strings.HasPrefix(r.URL.Path, path+testproj) {
			t.Errorf("Expected prefix %v, got %v", path+testproj, r.URL.Path)
		}
		if !(r.Header.Get("Content-Type") == string(ArchiveTypeGzip.MimeType) || r.Header.Get("Content-Type") == string(ArchiveTypeZip.MimeType)) {
			t.Errorf("Expected Content-Type: application/gzip or application/zip, got %v", r.Header.Get("Content-Type"))
		}
		w.WriteHeader(200)
	}))
	t.Cleanup(server.Close)

	uploadUrl := server.URL + path
	t.Run("upload tar with digest", func(t *testing.T) {
		url, err := uploadArchive(t.Context(), client.MockProvider{UploadUrl: uploadUrl}, testproj, &bytes.Buffer{}, ArchiveTypeGzip, digest)
		if err != nil {
			t.Fatalf("uploadArchive() failed: %v", err)
		}
		var expectedPath = path + testproj + "/" + digest + ArchiveTypeGzip.Extension
		if url != server.URL+expectedPath {
			t.Errorf("Expected %v, got %v", server.URL+expectedPath, url)
		}
	})

	t.Run("upload zip with digest", func(t *testing.T) {
		url, err := uploadArchive(t.Context(), client.MockProvider{UploadUrl: uploadUrl}, testproj, &bytes.Buffer{}, ArchiveTypeZip, digest)
		if err != nil {
			t.Fatalf("uploadArchive() failed: %v", err)
		}
		var expectedPath = path + testproj + "/" + digest + ArchiveTypeZip.Extension
		if url != server.URL+expectedPath {
			t.Errorf("Expected %v, got %v", server.URL+expectedPath, url)
		}
	})

	t.Run("upload with zip", func(t *testing.T) {
		url, err := uploadArchive(t.Context(), client.MockProvider{UploadUrl: uploadUrl}, testproj, &bytes.Buffer{}, ArchiveTypeZip, "")
		if err != nil {
			t.Fatalf("uploadContent() failed: %v", err)
		}
		var expectedPath = path + testproj + "/" + ArchiveTypeZip.Extension
		if url != server.URL+expectedPath {
			t.Errorf("Expected %v, got %v", server.URL+expectedPath, url)
		}
	})

	t.Run("upload with tar", func(t *testing.T) {
		url, err := uploadArchive(t.Context(), client.MockProvider{UploadUrl: uploadUrl}, testproj, &bytes.Buffer{}, ArchiveTypeGzip, "")
		if err != nil {
			t.Fatalf("uploadContent() failed: %v", err)
		}
		var expectedPath = path + testproj + "/" + ArchiveTypeGzip.Extension
		if url != server.URL+expectedPath {
			t.Errorf("Expected %v, got %v", server.URL+expectedPath, url)
		}
	})

	t.Run("force upload tar without digest", func(t *testing.T) {
		url, err := uploadArchive(t.Context(), client.MockProvider{UploadUrl: uploadUrl}, testproj, &bytes.Buffer{}, ArchiveTypeGzip, "")
		if err != nil {
			t.Fatalf("uploadArchive() failed: %v", err)
		}
		var expectedPath = path + testproj + "/" + ArchiveTypeGzip.Extension
		if url != server.URL+expectedPath {
			t.Errorf("Expected %v, got %v", server.URL+expectedPath, url)
		}
	})

	t.Run("force upload zip without digest", func(t *testing.T) {
		url, err := uploadArchive(t.Context(), client.MockProvider{UploadUrl: uploadUrl}, testproj, &bytes.Buffer{}, ArchiveTypeZip, "")
		if err != nil {
			t.Fatalf("uploadArchive() failed: %v", err)
		}
		var expectedPath = path + testproj + "/" + ArchiveTypeZip.Extension
		if url != server.URL+expectedPath {
			t.Errorf("Expected %v, got %v", server.URL+expectedPath, url)
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

		expected := []string{"Dockerfile", "altcomp.yaml", "compose.yaml.fixup", "compose.yaml.golden", "compose.yaml.warnings", "subdir", "subdir/subdir2", "subdir/subdir2/.gitkeep"}
		if !reflect.DeepEqual(files, expected) {
			t.Errorf("Expected files: %v, got %v", expected, files)
		}
	})
}

func Test_getRemoteBuildContext(t *testing.T) {
	tests := []struct {
		name       string
		uploadMode UploadMode
		expectUrl  string
		expectFile string
	}{
		{
			name:       "Default UploadMode",
			uploadMode: UploadModeDefault,
			expectUrl:  "https://mock-bucket.s3.amazonaws.com/project1/sha256-B+3Dq6U37SrlbnrfS4uIk3CDwrPJ+Q15TqUCPBEMQuA=.tar.gz", // same as Digest mode
			expectFile: "sha256-B+3Dq6U37SrlbnrfS4uIk3CDwrPJ+Q15TqUCPBEMQuA=.tar.gz",
		},
		{
			name:       "Force UploadMode",
			uploadMode: UploadModeForce,
			expectUrl:  "https://mock-bucket.s3.amazonaws.com/project1/.tar.gz", // server decides name
			expectFile: ".tar.gz",
		},
		{
			name:       "Digest UploadMode",
			uploadMode: UploadModeDigest,
			expectUrl:  "https://mock-bucket.s3.amazonaws.com/project1/sha256-B+3Dq6U37SrlbnrfS4uIk3CDwrPJ+Q15TqUCPBEMQuA=.tar.gz",
			expectFile: "sha256-B+3Dq6U37SrlbnrfS4uIk3CDwrPJ+Q15TqUCPBEMQuA=.tar.gz",
		},
		{
			name:       "Ignore UploadMode",
			uploadMode: UploadModeIgnore,
			expectUrl:  "$SRC/testdata/testproj", // show local paths in "defang config"
		},
		{
			name:       "Preview UploadMode",
			uploadMode: UploadModePreview,
			expectUrl:  "s3://cd-preview/sha256-B+3Dq6U37SrlbnrfS4uIk3CDwrPJ+Q15TqUCPBEMQuA=.tar.gz", // like digest but fake bucket
		},
		{
			name:       "Estimate UploadMode",
			uploadMode: UploadModeEstimate,
			expectUrl:  "s3://cd-preview/service1.tar.gz", // like preview but skip digest calculation
		},
	}

	tmpDir := t.TempDir()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PUT" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		defer r.Body.Close()
		if dst, err := os.Create(filepath.Join(tmpDir, path.Base(r.URL.Path))); err != nil {
			t.Errorf("Failed to create file: %v", err)
		} else {
			defer dst.Close()
			if _, err := io.Copy(dst, r.Body); err != nil {
				t.Errorf("Failed to write file: %v", err)
			}
		}
		w.WriteHeader(200)
	}))
	t.Cleanup(server.Close)

	src, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	normalizer := strings.NewReplacer(src, "$SRC", server.URL, "https://mock-bucket.s3.amazonaws.com")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := client.MockProvider{UploadUrl: server.URL}
			context := "../../../testdata/testproj"
			if err := standardizeDirMode(context); err != nil {
				t.Fatalf("Failed to standardize directory modes: %v", err)
			}
			url, err := getRemoteBuildContext(t.Context(), provider, "project1", "service1", &types.BuildConfig{
				Context: context,
			}, tt.uploadMode)
			if err != nil {
				t.Fatalf("getRemoteBuildContext() failed: %v", err)
			}
			if got := normalizer.Replace(url); got != tt.expectUrl {
				t.Errorf("Expected %v, got: %v", tt.expectUrl, got)
			}
			if tt.expectFile != "" {
				// Check that the file was uploaded correctly
				uploadedFile := filepath.Join(tmpDir, tt.expectFile)
				all, err := os.ReadFile(uploadedFile)
				if err != nil {
					t.Fatalf("Failed to read uploaded file %v: %v", uploadedFile, err)
				}
				if calcDigest(all) != "sha256-B+3Dq6U37SrlbnrfS4uIk3CDwrPJ+Q15TqUCPBEMQuA=" {
					t.Errorf("Uploaded file has unexpected digest: %v", calcDigest(all))
				}
			}
		})
	}
}

func standardizeDirMode(dir string) error {
	// Ensure root directory itself is 0755
	if err := os.Chmod(dir, 0755); err != nil {
		return fmt.Errorf("chmod root: %w", err)
	}

	return filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return os.Chmod(path, 0755)
		}

		return os.Chmod(path, 0644)
	})
}

func TestCreateTarballReader(t *testing.T) {
	t.Run("Default Dockerfile", func(t *testing.T) {
		buffer, err := createArchive(t.Context(), "../../../testdata/testproj", "", ArchiveTypeGzip)
		if err != nil {
			t.Fatalf("createTarballReader() failed: %v", err)
		}

		g, err := gzip.NewReader(buffer)
		if err != nil {
			t.Fatalf("gzip.NewReader() failed: %v", err)
		}
		t.Cleanup(func() { g.Close() })

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
		_, err := createArchive(t.Context(), "../../testdata", "Dockerfile.missing", ArchiveTypeGzip)
		if err == nil {
			t.Fatal("createTarballReader() should have failed")
		}
	})

	t.Run("Missing Context", func(t *testing.T) {
		_, err := createArchive(t.Context(), "asdfqwer", "", ArchiveTypeGzip)
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
			expectedFileName:  "",
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
			expectedFileName:  "",
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
				t.Errorf("Expected file name %s, but got: %s", tt.expectedFileName, fileName)
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
