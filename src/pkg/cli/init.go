package cli

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/http"
	"github.com/DefangLabs/defang/src/pkg/term"
)

type Sample struct {
	Name             string   `json:"name"`
	Title            string   `json:"title"`
	Category         string   `json:"category"` // Deprecated: use Languages instead
	Readme           string   `json:"readme"`   // unused
	DirectoryName    string   `json:"directoryName"`
	ShortDescription string   `json:"shortDescription"`
	Tags             []string `json:"tags"`
	Languages        []string `json:"languages"`
}

func FetchSamples(ctx context.Context) ([]Sample, error) {
	resp, err := http.GetWithHeader(ctx, "https://docs.defang.io/samples-v2.json", http.Header{"Accept-Encoding": []string{"gzip"}})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	term.Debug(resp.Header)
	reader := resp.Body
	if resp.Header.Get("Content-Encoding") == "gzip" {
		reader, err = gzip.NewReader(resp.Body)
		if err != nil {
			return nil, err
		}
		defer reader.Close()
	}
	var samples []Sample
	err = json.NewDecoder(reader).Decode(&samples)
	return samples, err
}

func InitFromSamples(ctx context.Context, dir string, names []string) error {
	const repo = "samples"
	const branch = "main"

	resp, err := http.GetWithContext(ctx, "https://github.com/DefangLabs/"+repo+"/archive/refs/heads/"+branch+".tar.gz")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	term.Debug(resp.Header)
	tarball, err := gzip.NewReader(resp.Body)
	if err != nil {
		return err
	}
	defer tarball.Close()
	tarReader := tar.NewReader(tarball)
	term.Info("Writing files to disk...")
	for {
		h, err := tarReader.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		for _, name := range names {
			prefix := fmt.Sprintf("%s-%s/samples/%s/", repo, branch, name)
			if base, ok := strings.CutPrefix(h.Name, prefix); ok && len(base) > 0 {
				fmt.Println("   -", base)
				path := filepath.Join(dir, base)
				if h.FileInfo().IsDir() {
					if err := os.MkdirAll(path, 0755); err != nil {
						return err
					}
					continue
				}
				if err := createFile(path, h, tarReader); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func createFile(base string, h *tar.Header, tarReader *tar.Reader) error {
	// Like os.Create, but with the same mode as the original file (so scripts are executable, etc.)
	file, err := os.OpenFile(base, os.O_RDWR|os.O_CREATE|os.O_EXCL, h.FileInfo().Mode())
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := io.Copy(file, tarReader); err != nil {
		return err
	}
	return nil
}
