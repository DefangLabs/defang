// Package oci reads public image metadata from OCI distribution registries.
package oci

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/distribution/reference"
)

const (
	mediaTypeOCIIndex          = "application/vnd.oci.image.index.v1+json"
	mediaTypeDockerManifestLst = "application/vnd.docker.distribution.manifest.list.v2+json"
	mediaTypeOCIManifest       = "application/vnd.oci.image.manifest.v1+json"
	mediaTypeDockerManifest    = "application/vnd.docker.distribution.manifest.v2+json"

	maxManifestBytes = 4 << 20
	maxConfigBytes   = 16 << 20
)

var manifestAccept = strings.Join([]string{mediaTypeOCIIndex, mediaTypeDockerManifestLst, mediaTypeOCIManifest, mediaTypeDockerManifest}, ", ")

// FetchImageLabels reads the config labels of a public image from its registry.
// It only ever authenticates anonymously, so a private image is an error rather
// than an opportunity to send credentials. For a multi-platform index the
// linux/amd64 image is used, which is the platform every CD driver runs.
func FetchImageLabels(ctx context.Context, httpClient *http.Client, image string) (map[string]string, error) {
	named, err := reference.ParseNormalizedNamed(image)
	if err != nil {
		return nil, fmt.Errorf("invalid image reference %q: %w", image, err)
	}
	reg := &registry{
		client: httpClient,
		host:   reference.Domain(named),
		repo:   reference.Path(named),
	}
	if reg.host == "docker.io" {
		reg.host = "registry-1.docker.io"
	}

	ref := "latest"
	var wantDigest string
	if digested, ok := named.(reference.Digested); ok {
		wantDigest = digested.Digest().String()
		ref = wantDigest
	} else if tagged, ok := named.(reference.Tagged); ok {
		ref = tagged.Tag()
	}

	manifest, err := reg.getManifest(ctx, ref, wantDigest)
	if err != nil {
		return nil, err
	}
	if len(manifest.Manifests) > 0 {
		platformDigest, err := selectPlatform(manifest.Manifests)
		if err != nil {
			return nil, err
		}
		if manifest, err = reg.getManifest(ctx, platformDigest, platformDigest); err != nil {
			return nil, err
		}
	}
	if manifest.Config.Digest == "" {
		return nil, fmt.Errorf("image %q: manifest has no config", image)
	}

	body, err := reg.get(ctx, "blobs/"+manifest.Config.Digest, "", maxConfigBytes)
	if err != nil {
		return nil, err
	}
	if err := verifyDigest(body, manifest.Config.Digest); err != nil {
		return nil, err
	}
	var config struct {
		Config struct {
			Labels map[string]string `json:"Labels"`
		} `json:"config"`
	}
	if err := json.Unmarshal(body, &config); err != nil {
		return nil, fmt.Errorf("image %q: invalid image config: %w", image, err)
	}
	return config.Config.Labels, nil
}

type descriptor struct {
	MediaType string `json:"mediaType"`
	Digest    string `json:"digest"`
	Platform  *struct {
		OS           string `json:"os"`
		Architecture string `json:"architecture"`
	} `json:"platform,omitempty"`
}

type manifest struct {
	Manifests []descriptor `json:"manifests"`
	Config    descriptor   `json:"config"`
}

func selectPlatform(manifests []descriptor) (string, error) {
	for _, m := range manifests {
		if m.Platform != nil && m.Platform.OS == "linux" && m.Platform.Architecture == "amd64" {
			return m.Digest, nil
		}
	}
	return "", errors.New("image index has no linux/amd64 image")
}

func verifyDigest(body []byte, digest string) error {
	algo, want, ok := strings.Cut(digest, ":")
	if !ok || algo != "sha256" {
		return fmt.Errorf("unsupported digest %q", digest)
	}
	sum := sha256.Sum256(body)
	if hex.EncodeToString(sum[:]) != want {
		return fmt.Errorf("content does not match digest %q", digest)
	}
	return nil
}

type registry struct {
	client *http.Client
	host   string
	repo   string
	token  string
}

func (r *registry) getManifest(ctx context.Context, ref, wantDigest string) (*manifest, error) {
	body, err := r.get(ctx, "manifests/"+ref, manifestAccept, maxManifestBytes)
	if err != nil {
		return nil, err
	}
	if wantDigest != "" {
		if err := verifyDigest(body, wantDigest); err != nil {
			return nil, err
		}
	}
	var m manifest
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, fmt.Errorf("invalid manifest for %s/%s:%s: %w", r.host, r.repo, ref, err)
	}
	return &m, nil
}

func (r *registry) get(ctx context.Context, path, accept string, limit int64) ([]byte, error) {
	resp, err := r.do(ctx, path, accept)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusUnauthorized && r.token == "" {
		challenge := resp.Header.Get("WWW-Authenticate")
		resp.Body.Close()
		if r.token, err = r.anonymousToken(ctx, challenge); err != nil {
			return nil, err
		}
		if resp, err = r.do(ctx, path, accept); err != nil {
			return nil, err
		}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s/%s/%s: %s", r.host, r.repo, path, resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("GET %s/%s/%s: response exceeds %d bytes", r.host, r.repo, path, limit)
	}
	return body, nil
}

func (r *registry) do(ctx context.Context, path, accept string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+r.host+"/v2/"+r.repo+"/"+path, nil)
	if err != nil {
		return nil, err
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	if r.token != "" {
		req.Header.Set("Authorization", "Bearer "+r.token)
	}
	return r.client.Do(req)
}

// anonymousToken requests a pull token without credentials, as described by
// https://distribution.github.io/distribution/spec/auth/token/.
func (r *registry) anonymousToken(ctx context.Context, challenge string) (string, error) {
	scheme, params, _ := strings.Cut(challenge, " ")
	if !strings.EqualFold(scheme, "Bearer") {
		return "", fmt.Errorf("%s/%s requires authentication", r.host, r.repo)
	}
	attrs := parseChallengeParams(params)
	realm, err := url.Parse(attrs["realm"])
	if err != nil || realm.Scheme != "https" {
		return "", fmt.Errorf("%s: invalid token realm %q", r.host, attrs["realm"])
	}
	query := realm.Query()
	if service := attrs["service"]; service != "" {
		query.Set("service", service)
	}
	query.Set("scope", "repository:"+r.repo+":pull")
	realm.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, realm.String(), nil)
	if err != nil {
		return "", err
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("anonymous token for %s/%s: %s", r.host, r.repo, resp.Status)
	}
	var token struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxManifestBytes)).Decode(&token); err != nil {
		return "", fmt.Errorf("anonymous token for %s/%s: %w", r.host, r.repo, err)
	}
	if token.Token != "" {
		return token.Token, nil
	}
	if token.AccessToken != "" {
		return token.AccessToken, nil
	}
	return "", fmt.Errorf("anonymous token for %s/%s: empty token", r.host, r.repo)
}

// parseChallengeParams parses the comma-separated key="value" pairs of a
// WWW-Authenticate challenge.
func parseChallengeParams(params string) map[string]string {
	attrs := map[string]string{}
	for params != "" {
		var key, value string
		key, params, _ = strings.Cut(strings.TrimLeft(params, " ,"), "=")
		if strings.HasPrefix(params, `"`) {
			value, params, _ = strings.Cut(params[1:], `"`)
		} else {
			value, params, _ = strings.Cut(params, ",")
		}
		attrs[strings.ToLower(strings.TrimSpace(key))] = value
	}
	return attrs
}
