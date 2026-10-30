package oci

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func digestOf(b []byte) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

type fakeRegistry struct {
	server      *httptest.Server
	blobs       map[string][]byte // path under /v2/defang/cd/ -> body
	requireAuth bool
	tokenCalls  int
	authHeaders []string
}

func newFakeRegistry(t *testing.T, labels map[string]string) (*fakeRegistry, string, string) {
	t.Helper()
	config, err := json.Marshal(map[string]any{"config": map[string]any{"Labels": labels}})
	require.NoError(t, err)
	amd64, err := json.Marshal(map[string]any{
		"mediaType": mediaTypeOCIManifest,
		"config":    map[string]any{"digest": digestOf(config)},
	})
	require.NoError(t, err)
	index, err := json.Marshal(map[string]any{
		"mediaType": mediaTypeOCIIndex,
		"manifests": []any{
			map[string]any{"digest": "sha256:attestation", "platform": map[string]any{"os": "unknown", "architecture": "unknown"}},
			map[string]any{"digest": digestOf(amd64), "platform": map[string]any{"os": "linux", "architecture": "amd64"}},
		},
	})
	require.NoError(t, err)

	r := &fakeRegistry{blobs: map[string][]byte{
		"manifests/" + digestOf(index): index,
		"manifests/v1-aws":             index,
		"manifests/" + digestOf(amd64): amd64,
		"blobs/" + digestOf(config):    config,
	}}
	r.server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/token" {
			r.tokenCalls++
			assert.Equal(t, "repository:defang/cd:pull", req.URL.Query().Get("scope"))
			assert.Equal(t, "fake", req.URL.Query().Get("service"))
			assert.Empty(t, req.Header.Get("Authorization"), "token requests must be anonymous")
			_, _ = w.Write([]byte(`{"token":"anon"}`))
			return
		}
		r.authHeaders = append(r.authHeaders, req.Header.Get("Authorization"))
		if r.requireAuth && req.Header.Get("Authorization") != "Bearer anon" {
			w.Header().Set("WWW-Authenticate", `Bearer realm="`+r.server.URL+`/token",service="fake",scope="repository:defang/cd:pull"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		body, ok := r.blobs[strings.TrimPrefix(req.URL.Path, "/v2/defang/cd/")]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(r.server.Close)
	return r, strings.TrimPrefix(r.server.URL, "https://") + "/defang/cd", digestOf(index)
}

func TestFetchImageLabels(t *testing.T) {
	labels := map[string]string{"io.defang.model-contract": "1"}

	t.Run("digest with anonymous token", func(t *testing.T) {
		reg, repo, digest := newFakeRegistry(t, labels)
		reg.requireAuth = true
		got, err := FetchImageLabels(t.Context(), reg.server.Client(), repo+"@"+digest)
		require.NoError(t, err)
		assert.Equal(t, labels, got)
		assert.Equal(t, 1, reg.tokenCalls)
	})

	t.Run("tag without auth", func(t *testing.T) {
		reg, repo, _ := newFakeRegistry(t, labels)
		got, err := FetchImageLabels(t.Context(), reg.server.Client(), repo+":v1-aws")
		require.NoError(t, err)
		assert.Equal(t, labels, got)
		assert.Equal(t, 0, reg.tokenCalls)
		for _, h := range reg.authHeaders {
			assert.Empty(t, h)
		}
	})

	t.Run("unlabeled image", func(t *testing.T) {
		reg, repo, digest := newFakeRegistry(t, nil)
		got, err := FetchImageLabels(t.Context(), reg.server.Client(), repo+"@"+digest)
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("digest mismatch", func(t *testing.T) {
		reg, repo, digest := newFakeRegistry(t, labels)
		reg.blobs["manifests/"+digest] = []byte(`{"manifests":[]}`)
		_, err := FetchImageLabels(t.Context(), reg.server.Client(), repo+"@"+digest)
		assert.ErrorContains(t, err, "does not match digest")
	})

	t.Run("missing image", func(t *testing.T) {
		reg, repo, _ := newFakeRegistry(t, labels)
		_, err := FetchImageLabels(t.Context(), reg.server.Client(), repo+":missing")
		assert.ErrorContains(t, err, "404")
	})

	t.Run("private image", func(t *testing.T) {
		reg, repo, digest := newFakeRegistry(t, labels)
		reg.server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("WWW-Authenticate", `Basic realm="private"`)
			w.WriteHeader(http.StatusUnauthorized)
		})
		_, err := FetchImageLabels(t.Context(), reg.server.Client(), repo+"@"+digest)
		assert.ErrorContains(t, err, "requires authentication")
	})

	t.Run("invalid reference", func(t *testing.T) {
		_, err := FetchImageLabels(t.Context(), http.DefaultClient, "Not A Ref")
		assert.ErrorContains(t, err, "invalid image reference")
	})
}

func TestParseChallengeParams(t *testing.T) {
	got := parseChallengeParams(`realm="https://auth.example/token",service="registry.example",scope="repository:a/b:pull"`)
	assert.Equal(t, map[string]string{
		"realm":   "https://auth.example/token",
		"service": "registry.example",
		"scope":   "repository:a/b:pull",
	}, got)
}
