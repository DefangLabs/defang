package cli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/bufbuild/connect-go"
	loader "github.com/compose-spec/compose-go/loader"
	"github.com/compose-spec/compose-go/types"
	pb "github.com/defang-io/defang/src/protos/io/defang/v1"
	"github.com/defang-io/defang/src/protos/io/defang/v1/defangv1connect"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

var (
	nonAlphanumeric = regexp.MustCompile(`[^a-zA-Z0-9]+`)
)

type ComposeError struct {
	error
}

func (e ComposeError) Unwrap() error {
	return e.error
}

func NormalizeServiceName(s string) string {
	return nonAlphanumeric.ReplaceAllLiteralString(strings.ToLower(s), "-")
}

func resolveEnv(k string) *string {
	// TODO: per spec, if the value is nil, then the value is taken from an interactive prompt
	v, ok := os.LookupEnv(k)
	if !ok {
		logrus.Warnf("environment variable not found: %q", k)
		// If the value could not be resolved, it should be removed
		return nil
	}
	return &v
}

func convertPlatform(platform string) pb.Platform {
	switch platform {
	default:
		logrus.Warnf("Unsupported platform: %q (assuming linux)", platform)
		fallthrough
	case "", "linux":
		return pb.Platform_LINUX_ANY
	case "linux/amd64":
		return pb.Platform_LINUX_AMD64
	case "linux/arm64", "linux/arm64/v8", "linux/arm64/v7", "linux/arm64/v6":
		return pb.Platform_LINUX_ARM64
	}
}

func loadDockerCompose(filePath, projectName string) (*types.Project, error) {
	Debug(" - Loading compose file", filePath)
	// Compose-go uses the logrus logger, so we need to configure it to be more like our own logger
	logrus.SetFormatter(&logrus.TextFormatter{DisableTimestamp: true, DisableColors: !DoColor, DisableLevelTruncation: true})
	project, err := loader.Load(types.ConfigDetails{
		WorkingDir:  filepath.Dir(filePath),
		ConfigFiles: []types.ConfigFile{{Filename: filePath}},
		Environment: map[string]string{}, // TODO: support environment variables?
	}, loader.WithDiscardEnvFiles, func(o *loader.Options) {
		o.SetProjectName(strings.ToLower(projectName), projectName != "") // normalize to lowercase
		o.SkipConsistencyCheck = true                                     // TODO: check fails if secrets are used but top-level 'secrets:' is missing
	})
	if err != nil {
		return nil, err
	}

	if DoVerbose {
		b, _ := yaml.Marshal(project)
		fmt.Println(string(b))
	}
	return project, nil
}

func getRemoteBuildContext(ctx context.Context, client defangv1connect.FabricControllerClient, name string, build *types.BuildConfig) (string, error) {
	root, err := filepath.Abs(build.Context)
	if err != nil {
		return "", fmt.Errorf("invalid build context: %w", err)
	}

	Info(" * Compressing build context for", name, "at", root)
	buffer, err := createTarballReader(ctx, build.Context, build.Dockerfile)
	if err != nil {
		return "", err
	}

	Info(" * Uploading build context for", name)
	return uploadTarball(ctx, client, buffer)
}

func convertPorts(ports []types.ServicePortConfig) ([]*pb.Port, error) {
	var pbports []*pb.Port
	for _, port := range ports {
		pbPort := &pb.Port{
			// Mode      string `yaml:",omitempty" json:"mode,omitempty"`
			// HostIP    string `mapstructure:"host_ip" yaml:"host_ip,omitempty" json:"host_ip,omitempty"`
			// Published string `yaml:",omitempty" json:"published,omitempty"`
			// Protocol  string `yaml:",omitempty" json:"protocol,omitempty"`
			Target: port.Target,
		}
		if port.Target < 1 || port.Target > 32767 {
			return nil, errors.New("port target must be an integer between 1 and 32767")
		}
		switch port.Protocol {
		case "":
			pbPort.Protocol = pb.Protocol_ANY // defaults to HTTP in CD
		case "tcp":
			pbPort.Protocol = pb.Protocol_TCP
		case "udp":
			pbPort.Protocol = pb.Protocol_UDP
		case "http":
			pbPort.Protocol = pb.Protocol_HTTP
		case "http2":
			pbPort.Protocol = pb.Protocol_HTTP2
		case "grpc":
			pbPort.Protocol = pb.Protocol_GRPC
		default:
			return nil, errors.New("port protocol not one of [tcp udp http http2 grpc]: " + port.Protocol)
		}
		switch port.Mode {
		case "":
			logrus.Warn("No port mode was specified; assuming 'host' (add 'mode' to silence)")
			fallthrough
		case "host":
			pbPort.Mode = pb.Mode_HOST
		case "ingress":
			pbPort.Mode = pb.Mode_INGRESS
			if pbPort.Protocol == pb.Protocol_TCP || pbPort.Protocol == pb.Protocol_UDP {
				logrus.Warn("TCP ingress is not supported; assuming HTTP")
				pbPort.Protocol = pb.Protocol_HTTP
			}
		default:
			return nil, errors.New("port mode not one of [host ingress]: " + port.Mode)
		}

		pbports = append(pbports, pbPort)
	}
	return pbports, nil
}

func uploadTarball(ctx context.Context, client defangv1connect.FabricControllerClient, body *bytes.Buffer) (string, error) {
	// Upload the tarball to the fabric controller storage TODO: use a streaming API
	digest := sha256.Sum256(body.Bytes())
	req := &pb.UploadURLRequest{
		Digest: "sha256-" + base64.StdEncoding.EncodeToString(digest[:]), // same as Nix
	}
	res, err := client.CreateUploadURL(ctx, connect.NewRequest(req))
	if err != nil {
		return "", err
	}

	if !DoDryRun {
		// Do an HTTP PUT to the generated URL
		req, err := http.NewRequestWithContext(ctx, "PUT", res.Msg.Url, body)
		if err != nil {
			return "", err
		}
		req.Header.Set("Content-Type", "application/gzip")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return "", fmt.Errorf("HTTP PUT failed with status code %v", resp.Status)
		}
	}

	// Remove query params from URL
	url, err := url.Parse(res.Msg.Url)
	if err != nil {
		return "", err
	}
	url.RawQuery = ""
	return url.String(), nil
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

func createTarballReader(ctx context.Context, root, dockerfile string) (*bytes.Buffer, error) {
	foundDockerfile := false
	if dockerfile == "" {
		dockerfile = "Dockerfile"
	}

	// TODO: use io.Pipe and do proper streaming (instead of buffering everything in memory)
	fileCount := 0
	var buf bytes.Buffer
	gzipWriter := &contextAwareWriter{ctx, gzip.NewWriter(&buf)}
	tarWriter := tar.NewWriter(gzipWriter)

	// Declare a map of files to ignore
	ignore := map[string]bool{
		".DS_Store":           true,
		".direnv":             true,
		".envrc":              true,
		".git":                true,
		".github":             true,
		".idea":               true,
		".vscode":             true,
		"__pycache__":         true,
		"defang":              true, // our binary
		"defang.exe":          true, // our binary
		"docker-compose.yml":  true,
		"docker-compose.yaml": true,
		"Dockerfile":          true, // overwritten below if specified
		"node_modules":        true,
		// ".dockerignore":       true, we're not using this, but Kaniko does
	}
	ignore[filepath.Base(dockerfile)] = false // always include the Dockerfile because Kaniko needs it
	// dockerignore.ReadAll(root) TODO: use this from "github.com/moby/buildkit/frontend/dockerfile/dockerignore"

	err := filepath.WalkDir(root, func(path string, de os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Ignore files in the ignore map
		if skip := ignore[de.Name()]; skip {
			if de.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Make sure the path is relative to the root
		relPath, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}

		Debug(" - Adding", relPath)

		info, err := de.Info()
		if err != nil {
			return err
		}

		header, err := tar.FileInfoHeader(info, info.Name())
		if err != nil {
			return err
		}

		// Make reproducible; WalkDir walks files in lexical order.
		header.ModTime = time.Unix(315532800, 0)
		header.Gid = 0
		header.Uid = 0
		header.Name = filepath.ToSlash(relPath)
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

		if !foundDockerfile && filepath.Clean(dockerfile) == relPath {
			foundDockerfile = true
		}

		fileCount++
		if fileCount == 11 {
			Warn(" ! The build context contains more than 10 files; press Ctrl+C if this is unexpected.")
		}

		_, err = io.Copy(tarWriter, file)
		if buf.Len() > 1024*1024*10 {
			return fmt.Errorf("build context is too large; this beta version is limited to 10MiB")
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

	if !foundDockerfile {
		return nil, fmt.Errorf("the specified dockerfile could not be read: %q", dockerfile)
	}

	return &buf, nil
}
