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
	"regexp"
	"strings"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
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

	// An empty digest is the "force" path: the caller wants a URL that has never
	// been used, so a redeploy of identical source still rebuilds. These used to
	// expect the bare extension (".tar.gz"), i.e. one shared blob for every forced
	// upload — which is exactly what made forced deploys reuse a stale image.
	for _, at := range []ArchiveType{ArchiveTypeGzip, ArchiveTypeZip} {
		t.Run("force upload without digest"+at.Extension, func(t *testing.T) {
			prefix := server.URL + path + testproj + "/"
			first, err := uploadArchive(t.Context(), client.MockProvider{UploadUrl: uploadUrl}, testproj, &bytes.Buffer{}, at, "")
			if err != nil {
				t.Fatalf("uploadArchive() failed: %v", err)
			}
			second, err := uploadArchive(t.Context(), client.MockProvider{UploadUrl: uploadUrl}, testproj, &bytes.Buffer{}, at, "")
			if err != nil {
				t.Fatalf("uploadArchive() failed: %v", err)
			}
			if first == second {
				t.Errorf("forced uploads reused %v; a repeated context URL makes the build a no-op", first)
			}
			for _, url := range []string{first, second} {
				if url == prefix+at.Extension {
					t.Errorf("forced upload used the shared fixed blob %v", url)
				}
				if !strings.HasPrefix(url, prefix) || !strings.HasSuffix(url, at.Extension) {
					t.Errorf("Expected %v<unique>%v, got %v", prefix, at.Extension, url)
				}
			}
		})
	}
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
		name        string
		uploadMode  UploadMode
		expectUrl   string
		expectUrlRe string
		expectFile  string
	}{
		{
			name:       "Default UploadMode",
			uploadMode: UploadModeDefault,
			expectUrl:  "https://mock-bucket.s3.amazonaws.com/project1/sha256-B+3Dq6U37SrlbnrfS4uIk3CDwrPJ+Q15TqUCPBEMQuA=.tar.gz", // same as Digest mode
			expectFile: "sha256-B+3Dq6U37SrlbnrfS4uIk3CDwrPJ+Q15TqUCPBEMQuA=.tar.gz",
		},
		{
			// Force must never reuse a URL: a repeated name makes the build context
			// identical to the previous deploy's, so the build is skipped and a stale
			// image ships. The name is a fresh UUID, so match a pattern, not a literal.
			name:        "Force UploadMode",
			uploadMode:  UploadModeForce,
			expectUrlRe: `^https://mock-bucket\.s3\.amazonaws\.com/project1/[0-9a-f-]{36}\.tar\.gz$`,
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

	tmpDir := t.TempDir() // change this to "/tmp" or so to inspect the files

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
			got := normalizer.Replace(url)
			if tt.expectUrlRe != "" {
				if !regexp.MustCompile(tt.expectUrlRe).MatchString(got) {
					t.Errorf("Expected URL matching %v, got: %v", tt.expectUrlRe, got)
				}
			} else if got != tt.expectUrl {
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

// TestForceUploadURLIsUniquePerCall pins the property that actually matters for
// UploadModeForce: two forced deploys of identical source must not reuse a URL.
// They previously both landed on ".tar.gz", so the build context was unchanged
// between deploys, the build was skipped as a no-op, and the old image kept
// running while the deploy reported success.
func TestForceUploadURLIsUniquePerCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body) //nolint:errcheck // discarding the upload body
		r.Body.Close()
		w.WriteHeader(200)
	}))
	t.Cleanup(server.Close)

	context := "../../../testdata/testproj"
	if err := standardizeDirMode(context); err != nil {
		t.Fatalf("Failed to standardize directory modes: %v", err)
	}
	provider := client.MockProvider{UploadUrl: server.URL}

	get := func() string {
		url, err := getRemoteBuildContext(t.Context(), provider, "project1", "service1",
			&types.BuildConfig{Context: context}, UploadModeForce)
		if err != nil {
			t.Fatalf("getRemoteBuildContext() failed: %v", err)
		}
		return url
	}

	first, second := get(), get()
	if first == second {
		t.Errorf("forced uploads reused the same URL %q; the build would be skipped as unchanged", first)
	}
	for _, u := range []string{first, second} {
		if strings.HasSuffix(u, "/.tar.gz") {
			t.Errorf("forced upload fell back to the shared fixed blob: %q", u)
		}
		if !strings.HasSuffix(u, ".tar.gz") {
			t.Errorf("forced upload lost its archive extension: %q", u)
		}
	}
}
