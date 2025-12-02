package cli

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/http"
	"github.com/DefangLabs/defang/src/pkg/term"
)

var ErrSampleNotFound = errors.New("sample not found")

type Sample struct {
	Name             string   `json:"name"`
	Title            string   `json:"title"`
	Category         string   `json:"category"` // Deprecated: use Languages instead
	Readme           string   `json:"readme"`   // unused
	DirectoryName    string   `json:"directoryName"`
	ShortDescription string   `json:"shortDescription"`
	Tags             []string `json:"tags"`
	Languages        []string `json:"languages"`
	Configs          []string `json:"configs"`
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

// MixinFromSamples copies the sample files into the given directory, skipping existing files.
func MixinFromSample(ctx context.Context, dir string, name string) error {
	return copyFromSamples(ctx, dir, []string{name}, true)
}

// InitFromSamples copies the sample(s) into the given directory, aborting if any files already exist.
func InitFromSamples(ctx context.Context, dir string, names []string) error {
	return copyFromSamples(ctx, dir, names, false)
}

func copyFromSamples(ctx context.Context, dir string, names []string, skipExisting bool) error {
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
		return fmt.Errorf("failed to read tarball: %w", err)
	}
	defer tarball.Close()
	tarReader := tar.NewReader(tarball)
	term.Info("Copying files to disk...")

	sampleFound := false

	for {
		h, err := tarReader.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		for _, name := range names {
			// Create the sample directory or subdirectory for each sample when there is more than one sample requested
			subdir := ""
			if len(names) > 1 {
				subdir = name
			}
			if err := os.MkdirAll(filepath.Join(dir, subdir), 0755); err != nil {
				return err
			}
			prefix := fmt.Sprintf("%s-%s/samples/%s/", repo, branch, name)
			if base, ok := strings.CutPrefix(h.Name, prefix); ok && len(base) > 0 {
				sampleFound = true
				term.Println("   -", base)
				path := filepath.Join(dir, subdir, base)
				if h.FileInfo().IsDir() {
					if err := os.MkdirAll(path, 0755); err != nil {
						return err
					}
					continue
				}
				// Use the same mode as the original file (so scripts are executable, etc.)
				if err := writeFileExcl(path, tarReader, h.FileInfo().Mode()); err != nil {
					if !skipExisting || !os.IsExist(err) {
						return err
					}
					term.Warnf("File already exists, skipping: %q", path)
				}
			}
		}
	}
	if !sampleFound {
		return ErrSampleNotFound
	}
	return nil
}

// writeFileExcl is like os.WriteFile, but with O_EXCL to avoid overwriting existing files.
func writeFileExcl(base string, reader io.Reader, mode os.FileMode) error {
	file, err := os.OpenFile(base, os.O_RDWR|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := io.Copy(file, reader); err != nil {
		return err
	}
	return nil
}
