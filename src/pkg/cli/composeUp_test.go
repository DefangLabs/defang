package cli

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func TestComposeUp(t *testing.T) {
	loader := compose.NewLoaderWithPath("../../tests/testproj/compose.yaml")
	proj, err := loader.LoadProject(context.Background())
	if err != nil {
		t.Fatalf("LoadProject() failed: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK) // same as S3
	}))
	defer server.Close()

	_, project, err := ComposeUp(context.Background(), client.MockClient{UploadUrl: server.URL + "/", Project: proj}, compose.BuildContextIgnore, defangv1.DeploymentMode_DEVELOPMENT)
	if !errors.Is(err, ErrDryRun) {
		t.Fatalf("ComposeUp() failed: %v", err)
	}
	if project == nil {
		t.Fatalf("ComposeUp() failed: project is nil")
	}
}
