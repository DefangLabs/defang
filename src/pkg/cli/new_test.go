package cli

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	ourHttp "github.com/DefangLabs/defang/src/pkg/http"
)

type mockRoundTripper struct{}

func (d mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	res := httptest.NewRecorder()
	if req.URL.String() != "https://github.com/DefangLabs/samples/archive/refs/heads/main.tar.gz" {
		res.Code = 404
	} else {
		gz := gzip.NewWriter(res.Body)
		tar.NewWriter(gz).Close()
		gz.Close()
	}
	return res.Result(), nil
}

func TestInitFromSamples(t *testing.T) {
	t.Run("mock", func(t *testing.T) {
		oldClient := ourHttp.DefaultClient
		t.Cleanup(func() { ourHttp.DefaultClient = oldClient })
		ourHttp.DefaultClient = &http.Client{Transport: mockRoundTripper{}}

		err := InitFromSamples(t.Context(), t.TempDir(), []string{"nonexisting"})
		if err == nil {
			t.Fatal("Expected test to fail")
		}
		if !errors.Is(err, ErrSampleNotFound) {
			t.Errorf("Expected error to be %v, got %v", ErrSampleNotFound, err)
		}
	})

	t.Run("wan", func(t *testing.T) {
		if testing.Short() {
			t.Skip("skipped; add -short to enable")
		}

		err := InitFromSamples(t.Context(), t.TempDir(), []string{"nonexisting"})
		if err == nil {
			t.Fatal("Expected test to fail")
		}
		if !errors.Is(err, ErrSampleNotFound) {
			t.Errorf("Expected error to be %v, got %v", ErrSampleNotFound, err)
		}
	})
}
