package compose

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/http"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/moby/patternmatcher"
	"github.com/moby/patternmatcher/ignorefile"
)

type BuildContext int

const (
	BuildContextDigest BuildContext = iota // the default: calculate the digest of the tarball so we can skip building the same image twice
	BuildContextForce                      // force: always upload the tarball, even if it's the same as a previous one
	BuildContextIgnore                     // dry-run: don't upload the tarball, just return the path
)

const (
	MiB                  = 1024 * 1024
	ContextFileLimit     = 100
	ContextSizeSoftLimit = 10 * MiB
	ContextSizeHardLimit = 100 * MiB
	sourceDateEpoch      = 315532800 // 1980-01-01, same as nix-shell
	defaultDockerIgnore  = `# Default .dockerignore file for Defang
**/.DS_Store
**/.direnv
**/.envrc
**/.git
**/.github
**/.idea
**/.next
**/.vscode
**/__pycache__
**/compose.yaml
**/compose.yml
**/defang.exe
**/docker-compose.yml
**/docker-compose.yaml
**/node_modules
**/Thumbs.db
# Ignore our own binary, but only in the root to avoid ignoring subfolders
defang`
)

func getRemoteBuildContext(ctx context.Context, client client.Client, name string, build *types.BuildConfig, force BuildContext) (string, error) {
	root, err := filepath.Abs(build.Context)
	if err != nil {
		return "", fmt.Errorf("invalid build context: %w", err) // already checked in ValidateProject
	}

	term.Info("Packaging the project files for", name, "at", root)
	buffer, err := createTarball(ctx, build.Context, build.Dockerfile)
	if err != nil {
		return "", err
	}

	var digest string
	if force == BuildContextDigest {
		// Calculate the digest of the tarball and pass it to the fabric controller (to avoid building the same image twice)
		sha := sha256.Sum256(buffer.Bytes())
		digest = "sha256-" + base64.StdEncoding.EncodeToString(sha[:]) // same as Nix
		term.Debug("Digest:", digest)
	}

	if force == BuildContextIgnore {
		return root, nil
	}

	term.Info("Uploading the project files for", name)
	return uploadTarball(ctx, client, buffer, digest)
}

func uploadTarball(ctx context.Context, client client.Client, body io.Reader, digest string) (string, error) {
	// Upload the tarball to the fabric controller storage;; TODO: use a streaming API
	ureq := &defangv1.UploadURLRequest{Digest: digest}
	res, err := client.CreateUploadURL(ctx, ureq)
	if err != nil {
		return "", err
	}

	// Do an HTTP PUT to the generated URL
	resp, err := http.Put(ctx, res.Url, "application/gzip", body)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP PUT failed with status code %v", resp.Status)
	}

	return http.RemoveQueryParam(res.Url), nil
}

type contextAwareWriter struct {
	ctx context.Context
	io.WriteCloser
}

func (cw contextAwareWriter) Write(p []byte) (n int, err error) {
	select {
	case <-cw.ctx.Done(): // Detect context cancelation
		return 0, cw.ctx.Err()
	default:
		return cw.WriteCloser.Write(p)
	}
}

func tryReadIgnoreFile(cwd, ignorefile string) io.ReadCloser {
	path := filepath.Join(cwd, ignorefile)
	reader, err := os.Open(path)
	if err != nil {
		return nil
	}
	term.Debug("Reading .dockerignore file from", ignorefile)
	return reader
}

func WalkContextFolder(root, dockerfile string, fn func(path string, de os.DirEntry, slashPath string) error) error {
	foundDockerfile := false
	if dockerfile == "" {
		dockerfile = "Dockerfile"
	} else {
		dockerfile = filepath.Clean(dockerfile)
	}

	// A Dockerfile-specific ignore-file takes precedence over the .dockerignore file at the root of the build context if both exist.
	dockerignore := dockerfile + ".dockerignore"
	reader := tryReadIgnoreFile(root, dockerignore)
	if reader == nil {
		dockerignore = ".dockerignore"
		reader = tryReadIgnoreFile(root, dockerignore)
		if reader == nil {
			term.Debug("No .dockerignore file found; using defaults")
			reader = io.NopCloser(strings.NewReader(defaultDockerIgnore))
		}
	}
	patterns, err := ignorefile.ReadAll(reader) // handles comments and empty lines
	reader.Close()
	if err != nil {
		return err
	}
	pm, err := patternmatcher.New(patterns)
	if err != nil {
		return err
	}

	err = filepath.WalkDir(root, func(path string, de os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Don't include the root directory itself in the tarball
		if path == root {
			return nil
		}

		// Make sure the path is relative to the root
		relPath, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}

		slashPath := filepath.ToSlash(relPath)

		// we need the Dockerfile, even if it's in the .dockerignore file
		if relPath == dockerfile {
			foundDockerfile = true
		} else if relPath == dockerignore {
			// we need the .dockerignore file too: it might ignore itself and/or the Dockerfile
		} else {
			// Ignore files using the dockerignore patternmatcher
			ignore, err := pm.MatchesOrParentMatches(slashPath) // always use forward slashes
			if err != nil {
				return err
			}
			if ignore {
				term.Debug("Ignoring", relPath) // TODO: avoid printing in this function
				if de.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}

		return fn(path, de, slashPath)
	})
	if err != nil {
		return err
	}

	if !foundDockerfile {
		return fmt.Errorf("the specified dockerfile could not be read: %q", dockerfile)
	}

	return nil

}

func createTarball(ctx context.Context, root, dockerfile string) (*bytes.Buffer, error) {
	fileCount := 0
	// TODO: use io.Pipe and do proper streaming (instead of buffering everything in memory)
	buf := &bytes.Buffer{}
	gzipWriter := &contextAwareWriter{ctx, gzip.NewWriter(buf)}
	tarWriter := tar.NewWriter(gzipWriter)

	doProgress := term.StdoutCanColor() && term.IsTerminal()
	err := WalkContextFolder(root, dockerfile, func(path string, de os.DirEntry, slashPath string) error {
		if term.DoDebug() {
			term.Debug("Adding", slashPath)
		} else if doProgress {
			term.Printf("%4d %s\r", fileCount, slashPath)
			defer term.ClearLine()
		}

		info, err := de.Info()
		if err != nil {
			return err
		}

		header, err := tar.FileInfoHeader(info, info.Name())
		if err != nil {
			return err
		}

		// Make reproducible; WalkDir walks files in lexical order.
		header.ModTime = time.Unix(sourceDateEpoch, 0)
		header.Gid = 0
		header.Uid = 0
		header.Name = slashPath
		err = tarWriter.WriteHeader(header)
		if err != nil {
			return err
		}

		if !info.Mode().IsRegular() {
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		fileCount++
		if fileCount == ContextFileLimit+1 {
			term.Warnf("The build context contains more than %d files; use --debug or create .dockerignore to exclude caches and build artifacts", ContextFileLimit)
		}

		bufLen := buf.Len()
		_, err = io.Copy(tarWriter, file)
		if buf.Len() > ContextSizeHardLimit {
			return fmt.Errorf("build context is too large; this beta version is limited to %dMiB, use --debug or create .dockerignore to exclude caches and build artifacts", ContextSizeHardLimit/MiB)
		}
		if bufLen <= ContextSizeSoftLimit && buf.Len() > ContextSizeSoftLimit {
			term.Warnf("The build context is more than %dMiB; use --debug or create .dockerignore to exclude caches and build artifacts", ContextSizeSoftLimit/MiB)
		}
		return err
	})

	if err != nil {
		return nil, err
	}

	// Close the tar and gzip writers before returning the buffer
	if err = tarWriter.Close(); err != nil {
		return nil, err
	}

	if err = gzipWriter.Close(); err != nil {
		return nil, err
	}

	return buf, nil
}
