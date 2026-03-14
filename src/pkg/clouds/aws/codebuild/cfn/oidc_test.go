package cfn

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchThumbprints(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			w.Write([]byte(`{"jwks_uri":"https://` + r.Host + `/jwks"}`))
		case "/jwks":
			w.Write([]byte(`{"keys":[{"x5c":[""],"x5t":"xw2OMQqwU1WYaVS09tc3_7r_y40"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	// Override the httpClient used in FetchThumbprints to accept the test server's self-signed TLS cert
	oldClient := httpClient
	httpClient = server.Client()
	t.Cleanup(func() {
		httpClient = oldClient
	})

	url := strings.TrimPrefix(server.URL, "https://")
	thumbprints, err := FetchThumbprints(url)
	if err != nil {
		t.Fatalf("FetchThumbprints failed: %v", err)
	}
	t.Logf("Fetched thumbprints: %v", thumbprints)
	if len(thumbprints) == 0 {
		t.Fatalf("expected at least one thumbprint, got none")
	}
	const expectedLen = 40
	for i, tp := range thumbprints {
		if len(tp) != expectedLen {
			t.Errorf("thumbprint %d is of unexpected length %d, expected %d", i, len(tp), expectedLen)
		}
	}
}
