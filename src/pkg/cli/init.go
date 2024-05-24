package cli

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
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

func InitFromSample(ctx context.Context, name string) error {
	const repo = "samples"
	const branch = "main"

	prefix := fmt.Sprintf("%s-%s/samples/%s/", repo, branch, name)
	resp, err := http.GetWithContext(ctx, "https://github.com/DefangLabs/"+repo+"/archive/refs/heads/"+branch+".tar.gz")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	term.Debug(resp.Header)
	body, err := gzip.NewReader(resp.Body)
	if err != nil {
		return err
	}
	defer body.Close()
	tarReader := tar.NewReader(body)
	term.Info(" * Writing files to disk...")
	for {
		h, err := tarReader.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		if base, ok := strings.CutPrefix(h.Name, prefix); ok && len(base) > 0 {
			fmt.Println("   -", base)
			if h.FileInfo().IsDir() {
				if err := os.MkdirAll(base, 0755); err != nil {
					return err
				}
				continue
			}
			// Like os.Create, but with the same mode as the original file (so scripts are executable, etc.)
			file, err := os.OpenFile(base, os.O_RDWR|os.O_CREATE|os.O_TRUNC, h.FileInfo().Mode())
			if err != nil {
				return err
			}
			if _, err := io.Copy(file, tarReader); err != nil {
				return err
			}
		}
	}
	return nil
}
