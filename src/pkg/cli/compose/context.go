package compose

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/http"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/go-units"
	"github.com/moby/patternmatcher"
	"github.com/moby/patternmatcher/ignorefile"
)

type UploadMode int

const (
	UploadModeDigest   UploadMode = iota // the default: calculate the digest of the tarball so we can skip building the same image twice
	UploadModeForce                      // force: always upload the tarball, even if it's the same as a previous one
	UploadModeIgnore                     // dry-run: don't upload the tarball, just return the path
	UploadModePreview                    // preview: like dry-run but does start the preview command
	UploadModeEstimate                   // cost estimation: like preview, but skips the tarball
)

const (
	MiB                         = 1024 * 1024
	ContextFileLimit            = 100
	ContextSizeSoftLimit        = 10 * MiB
	DefaultContextSizeHardLimit = 100 * MiB

	sourceDateEpoch = 315532800 // 1980-01-01, same as nix-shell
	dotdockerignore = ".dockerignore"
	// The default .dockerignore for projects that don't have one. Keep in sync with upload.ts in pulumi-defang repo.
	defaultDockerIgnore = `# Default .dockerignore file for Defang
**/__pycache__
**/.direnv
**/.DS_Store
**/.envrc
**/.git
**/.github
**/.idea
**/.next
**/.vscode
**/compose.*.yaml
**/compose.*.yml
**/compose.yaml
**/compose.yml
**/docker-compose.*.yaml
**/docker-compose.*.yml
**/docker-compose.yaml
**/docker-compose.yml
**/node_modules
**/Thumbs.db
Dockerfile
*.Dockerfile
# Ignore our own binary, but only in the root to avoid ignoring subfolders
defang
defang.exe
# Ignore our project-level state
.defang`
)

type ArchiveType string

const ArchiveTypeZip ArchiveType = "application/zip"
const ArchiveTypeGzip ArchiveType = "application/gzip"

type WriterFactory interface {
	CreateHeader(info fs.FileInfo, slashPath string) (io.Writer, error)
	Close() error
}

type tarFactory struct {
	*tar.Writer
	gzipWriter io.WriteCloser
}

func (tw *tarFactory) CreateHeader(info fs.FileInfo, slashPath string) (io.Writer, error) {
	// Convert zip header to tar header
	header, err := tar.FileInfoHeader(info, info.Name())
	if err != nil {
		return nil, err
	}

	if !info.Mode().IsRegular() {
		return nil, nil
	}

	// Make reproducible; WalkDir walks files in lexical order.
	header.ModTime = time.Unix(sourceDateEpoch, 0)
	header.Gid = 0
	header.Uid = 0
	header.Name = slashPath
	err = tw.WriteHeader(header)
	return tw.Writer, err
}

func (tw *tarFactory) Close() error {
	// Close the tar and gzip writers before returning the buffer
	err := tw.Writer.Close()
	if err != nil {
		return err
	}

	err = tw.gzipWriter.Close()
	if err != nil {
		return err
	}

	return nil
}

type zipFactory struct {
	*zip.Writer
}

func (zw *zipFactory) CreateHeader(info fs.FileInfo, slashPath string) (io.Writer, error) {
	// Create a new zip file header
	header := &zip.FileHeader{
		Name:   slashPath,
		Method: zip.Deflate,
	}

	// Make reproducible
	header.Modified = time.Unix(sourceDateEpoch, 0)

	if !info.Mode().IsRegular() {
		if info.IsDir() {
			header.Name = slashPath + "/" // Ensure directory paths end with slash
			header.Method = zip.Store     // Directories are stored without compression
			_, err := zw.Writer.CreateHeader(header)
			return nil, err
		}
		return nil, nil
	}

	// Create file entry in zip
	writer, err := zw.Writer.CreateHeader(header)
	return writer, err
}

func (zw *zipFactory) Close() error {
	// Close the zip writer before returning the buffer
	err := zw.Writer.Close()
	if err != nil {
		return err
	}
	return nil
}

func parseContextLimit(limit string, def int64) int64 {
	if size, err := units.RAMInBytes(limit); err == nil {
		return size
	}
	return def
}

var (
	ContextSizeHardLimit = parseContextLimit(os.Getenv("DEFANG_BUILD_CONTEXT_LIMIT"), DefaultContextSizeHardLimit)
)

func getRemoteBuildContext(ctx context.Context, provider client.Provider, project, name string, build *types.BuildConfig, upload UploadMode) (string, error) {
	root, err := filepath.Abs(build.Context)
	if err != nil {
		return "", fmt.Errorf("invalid build context: %w", err) // already checked in ValidateProject
	}

	var archiveType ArchiveType
	// If we have a Railpack build, we use a zip archive
	if build.Dockerfile == RAILPACK {
		archiveType = ArchiveTypeZip
		// We use tar for all other builds
	} else {
		archiveType = ArchiveTypeGzip
	}

	switch upload {
	case UploadModeIgnore:
		// `compose config`, ie. dry-run: don't upload the archive, just return the path as-is
		return root, nil
	case UploadModeEstimate:
		// For estimation, we don't bother packaging the files, we just return a placeholder URL
		return fmt.Sprintf("s3://cd-preview/%v", time.Now().Unix()), nil
	}

	term.Info("Packaging the project files for", name, "at", root)
	buffer, err := createArchive(ctx, build.Context, build.Dockerfile, archiveType)
	if err != nil {
		return "", err
	}

	var digest string
	switch upload {
	case UploadModeDigest:
		// Calculate the digest of the tarball and pass it to the fabric controller (to avoid building the same image twice)
		sha := sha256.Sum256(buffer.Bytes())
		digest = "sha256-" + base64.StdEncoding.EncodeToString(sha[:]) // same as Nix
		term.Debug("Digest:", digest)
	case UploadModePreview:
		// For preview, we invoke the CD "preview" command, which will want a valid (S3) URL, even though it won't be used
		return fmt.Sprintf("s3://cd-preview/%v", time.Now().Unix()), nil
	case UploadModeForce:
		// Force: always upload the tarball (to a random URL), triggering a new build
	default:
		panic("unexpected UploadMode value")
	}

	term.Info("Uploading the project files for", name)
	return uploadArchive(ctx, provider, project, buffer, archiveType, digest)
}

func uploadArchive(ctx context.Context, provider client.Provider, project string, body io.Reader, contentType ArchiveType, digest string) (string, error) {
	// Upload the archive to the fabric controller storage;; TODO: use a streaming API
	ureq := &defangv1.UploadURLRequest{Digest: digest, Project: project}
	res, err := provider.CreateUploadURL(ctx, ureq)
	if err != nil {
		return "", err
	}

	// Do an HTTP PUT to the generated URL
	resp, err := http.Put(ctx, res.Url, string(contentType), body)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP PUT failed with status code %v", resp.Status)
	}

	url := http.RemoveQueryParam(res.Url)
	const gcpPrefix = "https://storage.googleapis.com/"
	if strings.HasPrefix(url, gcpPrefix) {
		url = "gs://" + url[len(gcpPrefix):]
	}
	return url, nil
}

type contextAwareReader struct {
	ctx context.Context
	io.ReadCloser
}

func (cr contextAwareReader) Read(p []byte) (n int, err error) {
	select {
	case <-cr.ctx.Done(): // Detect context cancelation
		return 0, cr.ctx.Err()
	default:
		return cr.ReadCloser.Read(p)
	}
}

// tryReadIgnoreFile attempts to read the specified ignore file.
func tryReadIgnoreFile(cwd, ignorefile string) io.ReadCloser {
	path := filepath.Join(cwd, ignorefile)
	reader, err := os.Open(path)
	if err != nil {
		return nil
	}
	term.Debug("Reading .dockerignore file from", ignorefile)
	return reader
}

// writeDefaultIgnoreFile writes a default
// .dockerignore file to the specified directory.
// Returns the filename of the written file and an error.
func writeDefaultIgnoreFile(cwd string) (string, error) {
	path := filepath.Join(cwd, dotdockerignore)
	term.Debug("Writing .dockerignore file to", path)

	err := os.WriteFile(path, []byte(defaultDockerIgnore), 0644)
	if err != nil {
		return "", fmt.Errorf("failed to write default .dockerignore file: %w", err)
	}

	return dotdockerignore, nil
}

// getDockerIgnorePatterns attempts to read the ignore file
// for the specified Dockerfile and returns the patterns and
// the name of the ignore file.
func getDockerIgnorePatterns(root, dockerfile string) ([]string, string, error) {
	// Check for Dockerfile-specific ignore file
	// Attempt to read Dockerfile-specific ignore file
	dockerignore := dockerfile + dotdockerignore
	reader := tryReadIgnoreFile(root, dockerignore)
	if reader == nil {
		// Fallback to .dockerignore
		dockerignore = dotdockerignore
		reader = tryReadIgnoreFile(root, dockerignore)
		if reader == nil {
			// Generate a default .dockerignore file if none exists
			term.Warn("No .dockerignore file found; generating default .dockerignore")
			var err error
			dockerignore, err = writeDefaultIgnoreFile(root)
			if err != nil {
				return nil, "", fmt.Errorf("failed to write default .dockerignore file: %w", err)
			}
			// Try reading the newly created .dockerignore file
			reader = tryReadIgnoreFile(root, dockerignore)
			if reader == nil {
				return nil, "", fmt.Errorf("failed to read default .dockerignore file: %s at the path: %s", dockerignore, root)
			}
		}
	}

	defer reader.Close()

	patterns, err := ignorefile.ReadAll(reader) // handles comments and empty lines
	if err != nil {
		return nil, "", fmt.Errorf("failed to read ignore file: %w", err)
	}

	return patterns, dockerignore, nil
}

func WalkContextFolder(root, dockerfile string, fn func(path string, de os.DirEntry, slashPath string) error) error {
	if dockerfile == "" {
		dockerfile = "Dockerfile"
	} else {
		dockerfile = filepath.Clean(dockerfile)
	}

	// Get the ignore patterns from the .dockerignore file
	patterns, dockerignore, err := getDockerIgnorePatterns(root, dockerfile)
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
		} else if relPath == dockerignore {
			// we need the .dockerignore file too: it might ignore itself and/or the Dockerfile, but is needed by the builder
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

	return nil
}

func createArchive(ctx context.Context, root string, dockerfile string, contentType ArchiveType) (*bytes.Buffer, error) {
	fileCount := 0
	// TODO: use io.Pipe and do proper streaming (instead of buffering everything in memory)

	buf := &bytes.Buffer{}
	var factory WriterFactory
	if contentType == ArchiveTypeZip {
		zipWriter := zip.NewWriter(buf)
		factory = &zipFactory{zipWriter}
	} else {
		gzipWriter := gzip.NewWriter(buf)
		tarWriter := tar.NewWriter(gzipWriter)
		factory = &tarFactory{tarWriter, gzipWriter}
	}

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

		writer, err := factory.CreateHeader(info, slashPath)
		if err != nil || writer == nil {
			return err
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		// Wrap the file reader with context-aware reader
		contextReader := &contextAwareReader{ctx, file}

		fileCount++
		if fileCount == ContextFileLimit+1 {
			term.Warnf("the build context contains more than %d files; use --debug or create .dockerignore to exclude caches and build artifacts", ContextFileLimit)
		}

		bufLen := buf.Len()
		_, err = io.Copy(writer, contextReader)
		if int64(buf.Len()) > ContextSizeHardLimit {
			return fmt.Errorf("the build context is limited to %s; consider downloading large files in the Dockerfile or set the DEFANG_BUILD_CONTEXT_LIMIT environment variable", units.BytesSize(float64(ContextSizeHardLimit)))
		}
		if bufLen <= ContextSizeSoftLimit && buf.Len() > ContextSizeSoftLimit {
			term.Warnf("the build context is larger than %s; use --debug or create .dockerignore to exclude caches and build artifacts", units.BytesSize(float64(buf.Len())))
		}
		return err
	})

	if err != nil {
		return nil, err
	}

	err = factory.Close() // Close the tar or zip writer
	if err != nil {
		return nil, err
	}

	return buf, nil
}
