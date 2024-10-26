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
	"reflect"
	"strings"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
)

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
		url, err := uploadTarball(context.Background(), client.MockProvider{UploadUrl: server.URL + path}, &bytes.Buffer{}, digest)
		if err != nil {
			t.Fatalf("uploadTarball() failed: %v", err)
		}
		const expectedPath = path + digest
		if url != server.URL+expectedPath {
			t.Errorf("Expected %v, got %v", server.URL+expectedPath, url)
		}
	})

	t.Run("force upload without digest", func(t *testing.T) {
		url, err := uploadTarball(context.Background(), client.MockProvider{UploadUrl: server.URL + path}, &bytes.Buffer{}, "")
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
		err := WalkContextFolder("../../../tests/testproj", "", func(path string, de os.DirEntry, slashPath string) error {
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
		err := WalkContextFolder("../../tests", "Dockerfile.missing", func(string, os.DirEntry, string) error { return nil })
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
}

func TestCreateTarballReader(t *testing.T) {
	t.Run("Default Dockerfile", func(t *testing.T) {
		buffer, err := createTarball(context.Background(), "../../../tests/testproj", "")
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
