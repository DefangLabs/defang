package cli

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
)

func TestComposeStart(t *testing.T) {
	DoDryRun = true
	defer func() { DoDryRun = false }()

	loader := compose.Loader{"../../tests/testproj/compose.yaml"}
	proj, err := loader.LoadCompose(context.Background())
	if err != nil {
		t.Fatalf("LoadCompose() failed: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	_, err = ComposeStart(context.Background(), client.MockClient{UploadUrl: server.URL + "/", Project: proj}, false)
	if !errors.Is(err, ErrDryRun) {
		t.Fatalf("ComposeStart() failed: %v", err)
	}
}
