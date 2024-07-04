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
	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/types"
)

type deployMock struct {
	client.MockClient
}

func (d deployMock) Deploy(ctx context.Context, req *defangv1.DeployRequest) (*defangv1.DeployResponse, error) {
	if req.Compose == nil || req.Services == nil {
		return nil, errors.New("invalid request")
	}

	asMap := req.Compose.AsMap()
	_, err := loader.LoadWithContext(ctx, types.ConfigDetails{ConfigFiles: []types.ConfigFile{{Config: asMap}}}, func(o *loader.Options) {
		o.SetProjectName(asMap["name"].(string), true) // HACK: workaround for bug in compose-go where it insists on loading the project name from the file
	})
	if err != nil {
		return nil, err
	}

	return &defangv1.DeployResponse{}, nil
}

func TestComposeUp(t *testing.T) {
	loader := compose.NewLoaderWithPath("../../tests/testproj/compose.yaml")
	proj, err := loader.LoadProject(context.Background())
	if err != nil {
		t.Fatalf("LoadProject() failed: %v", err)
	}

	gotContext := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("ComposeStart() failed: expected PUT request, got %s", r.Method)
		}
		gotContext = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	_, project, err := ComposeUp(context.Background(), deployMock{MockClient: client.MockClient{UploadUrl: server.URL + "/", Project: proj}}, false, defangv1.DeploymentMode_DEVELOPMENT)
	if err != nil {
		t.Fatalf("ComposeUp() failed: %v", err)
	}
	if project == nil {
		t.Fatalf("ComposeUp() failed: project is nil")
	}
	if !gotContext {
		t.Errorf("ComposeStart() failed: did not get context")
	}
}
