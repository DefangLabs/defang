package github

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	ourHttp "github.com/DefangLabs/defang/src/pkg/http"
)

type mockRoundTripper struct {
	method string
	url    string
	resp   *http.Response
}

func (rt *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if rt.method != "" && rt.method != req.Method {
		return nil, fmt.Errorf("expected method %q; got %q", rt.method, req.Method)
	}
	if rt.url != "" && rt.url != req.URL.String() {
		return nil, fmt.Errorf("expected URL %q; got %q", rt.url, req.URL.String())
	}
	return rt.resp, nil
}

func TestGetLatestVersion(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping GitHub HTTP test in short mode to avoid rate limits.")
	}

	ctx := t.Context()

	const version = "v1.2.3"
	rec := httptest.NewRecorder()
	rec.Header().Add("Content-Type", "application/json")
	rec.WriteString(fmt.Sprintf(`{"tag_name":"%v"}`, version))
	response := rec.Result()

	client := ourHttp.DefaultClient
	t.Cleanup(func() {
		ourHttp.DefaultClient = client
		response.Body.Close()
	})

	ourHttp.DefaultClient = &http.Client{Transport: &mockRoundTripper{
		method: http.MethodGet,
		url:    "https://api.github.com/repos/DefangLabs/defang/releases/latest",
		resp:   response,
	}}

	v, err := GetLatestReleaseTag(ctx)
	if err != nil {
		t.Fatalf("GetLatestVersion() error = %v; want nil", err)
	}
	if v == "" {
		t.Fatalf("GetLatestVersion() = %v; want non-empty", v)
	}
	if v != version {
		t.Errorf("GetLatestVersion() = %v; want %v", v, version)
	}
}
