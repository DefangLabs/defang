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

func TestComposeUp(t *testing.T) {
	DoDryRun = true
	defer func() { DoDryRun = false }()

	loader := compose.NewLoader("../../tests/testproj/compose.yaml")
	proj, err := loader.LoadCompose(context.Background())
	if err != nil {
		t.Fatalf("LoadCompose() failed: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	_, project, err := ComposeUp(context.Background(), client.MockClient{UploadUrl: server.URL + "/", Project: proj}, false)
	if !errors.Is(err, ErrDryRun) {
		t.Fatalf("ComposeUp() failed: %v", err)
	}
	if project == nil {
		t.Fatalf("ComposeUp() failed: project is nil")
	}
}
