package byoc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// cdRegistry serves one single-platform CD image per label value at <host>/cd:<tag>.
func cdRegistry(t *testing.T, labelsByTag map[string]map[string]string) *httptest.Server {
	t.Helper()
	blobs := map[string][]byte{}
	for tag, labels := range labelsByTag {
		config, err := json.Marshal(map[string]any{"config": map[string]any{"Labels": labels}})
		require.NoError(t, err)
		sum := sha256.Sum256(config)
		digest := "sha256:" + hex.EncodeToString(sum[:])
		manifest, err := json.Marshal(map[string]any{"config": map[string]any{"digest": digest}})
		require.NoError(t, err)
		blobs["/v2/cd/manifests/"+tag] = manifest
		manifestSum := sha256.Sum256(manifest)
		blobs["/v2/cd/manifests/sha256:"+hex.EncodeToString(manifestSum[:])] = manifest
		blobs["/v2/cd/blobs/"+digest] = config
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if body, ok := blobs[r.URL.Path]; ok {
			_, _ = w.Write(body)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(server.Close)
	return server
}

func digestRef(t *testing.T, server *httptest.Server, tag string) string {
	t.Helper()
	resp, err := server.Client().Get(server.URL + "/v2/cd/manifests/" + tag)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func TestVerifyModelContract(t *testing.T) {
	server := cdRegistry(t, map[string]map[string]string{
		"new":    {modelContractLabel: "1"},
		"old":    nil,
		"future": {modelContractLabel: "2"},
	})
	host := strings.TrimPrefix(server.URL, "https://")

	tests := []struct {
		name     string
		image    string
		override string
		wantErr  string
	}{
		{name: "labeled image", image: host + "/cd:new"},
		{name: "unlabeled pinned image", image: host + "/cd:old", wantErr: "does not support managed model requests (it needs the io.defang.model-contract=1 image label). If this stack is pinned to the CD engine of an earlier deployment, re-run with --allow-upgrade"},
		{name: "unlabeled override", image: host + "/cd:old", override: host + "/cd:old", wantErr: "DEFANG_CD_IMAGE is set: unset it, or point it at a CD image that supports managed models"},
		{name: "unreadable image", image: host + "/cd:missing", wantErr: "could not be verified"},
		{name: "newer contract", image: host + "/cd:future", wantErr: "implements model contract 2, but this Defang CLI requires 1; update the Defang CLI"},
		{name: "no image", wantErr: `CD image "" is not set`},
		{name: "private registry digest verified on the public repo", image: "private.example/cd@" + digestRef(t, server, "new")},
		{name: "private registry digest of an old image", image: "private.example/cd@" + digestRef(t, server, "old"), wantErr: "does not support managed model requests"},
		{name: "private registry tag cannot be verified", image: "private.example/cd:new", wantErr: "could not be verified"},
	}
	saved := publicCDRepo
	publicCDRepo = host + "/cd"
	t.Cleanup(func() { publicCDRepo = saved })
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("DEFANG_CD_IMAGE", tt.override)
			err := verifyModelContract(t.Context(), server.Client(), tt.image)
			if tt.wantErr == "" {
				assert.NoError(t, err)
			} else {
				assert.ErrorContains(t, err, tt.wantErr)
			}
		})
	}
}

type canIUseFabric struct {
	client.MockFabricClient
	cdImage string
}

func (f canIUseFabric) CanIUse(context.Context, *defangv1.CanIUseRequest) (*defangv1.CanIUseResponse, error) {
	return &defangv1.CanIUseResponse{CdImage: f.cdImage}, nil
}

type pinnedProvider struct {
	client.Provider
	base       *ByocBaseClient
	previousCD string
}

func (pinnedProvider) AccountInfo(context.Context) (*client.AccountInfo, error) {
	return &client.AccountInfo{Provider: client.ProviderAWS, Region: "us-west-2"}, nil
}

func (pinnedProvider) Driver() string { return "test" }

func (p pinnedProvider) GetProjectUpdate(context.Context, string) (*defangv1.ProjectUpdate, error) {
	return &defangv1.ProjectUpdate{CdVersion: p.previousCD}, nil
}

func (p pinnedProvider) GetStackName() string { return p.base.GetStackName() }

func (p pinnedProvider) SetCanIUseConfig(resp *defangv1.CanIUseResponse) {
	p.base.SetCanIUseConfig(resp)
}

// The gate must check the image CanIUseProvider selected, not Fabric's latest.
func TestVerifyModelContractChecksSelectedImage(t *testing.T) {
	server := cdRegistry(t, map[string]map[string]string{"new": {modelContractLabel: "1"}, "old": nil})
	host := strings.TrimPrefix(server.URL, "https://")
	saved := modelContractHTTPClient
	modelContractHTTPClient = server.Client()
	t.Cleanup(func() { modelContractHTTPClient = saved })

	tests := []struct {
		name         string
		override     string
		allowUpgrade bool
		wantErr      string
	}{
		{name: "pinned to the previous engine", wantErr: "--allow-upgrade"},
		{name: "upgrade to the latest engine", allowUpgrade: true},
		{name: "override to an old engine", override: host + "/cd:old", allowUpgrade: true, wantErr: "DEFANG_CD_IMAGE is set"},
		{name: "override to a new engine", override: host + "/cd:new"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("DEFANG_CD_IMAGE", tt.override)
			provider := pinnedProvider{base: NewByocBaseClient("tenant", nil, "beta"), previousCD: host + "/cd:old"}
			fabric := canIUseFabric{cdImage: host + "/cd:new"}
			require.NoError(t, client.CanIUseProvider(t.Context(), fabric, provider, "project", 1, tt.allowUpgrade))

			err := provider.base.VerifyModelContract(t.Context())
			if tt.wantErr == "" {
				assert.NoError(t, err)
			} else {
				assert.ErrorContains(t, err, tt.wantErr)
			}
		})
	}
}
