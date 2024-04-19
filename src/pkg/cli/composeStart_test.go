package cli

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/defang-io/defang/src/pkg/cli/client"
)

func TestComposeStart(t *testing.T) {
	DoDryRun = true
	defer func() { DoDryRun = false }()

	project, err := LoadComposeWithProjectName("../../tests/testproj/compose.yaml", "tenant-id")
	if err != nil {
		t.Fatalf("LoadComposeWithProjectName() failed: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	_, err = ComposeStart(context.Background(), client.MockClient{UploadUrl: server.URL + "/"}, project, false)
	if !errors.Is(err, ErrDryRun) {
		t.Fatalf("ComposeStart() failed: %v", err)
	}
}
