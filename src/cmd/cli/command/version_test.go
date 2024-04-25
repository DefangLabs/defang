package command

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetCurrentVersion(t *testing.T) {
	dev := GetCurrentVersion()
	if dev != "development" {
		t.Errorf("GetCurrentVersion() = %v; want development", dev)
	}
	version = "1.0.0"
	v := GetCurrentVersion()
	if v != "v1.0.0" {
		t.Errorf("GetCurrentVersion() = %v; want v1.0.0", v)
	}
}

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
	ctx := context.Background()

	const version = "v1.2.3"
	rec := httptest.NewRecorder()
	rec.Header().Add("Content-Type", "application/json")
	rec.WriteString(fmt.Sprintf(`{"tag_name":"%v"}`, version))

	httpClient = &http.Client{Transport: &mockRoundTripper{
		method: http.MethodGet,
		url:    "https://api.github.com/repos/defang-io/defang/releases/latest",
		resp:   rec.Result(),
	}}

	v, err := GetLatestVersion(ctx)
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
