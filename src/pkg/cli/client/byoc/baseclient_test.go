package byoc

import (
	"context"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/compose-spec/compose-go/v2/loader"
)

type dummyLister struct{}

func (dummyLister) BootstrapList(context.Context) ([]string, error) {
	return nil, nil
}

func TestLoadProjectName(t *testing.T) {
	t.Run("no compose file but COMPOSE_PROJECT_NAME set", func(t *testing.T) {
		t.Setenv("COMPOSE_PROJECT_NAME", "asdf")
		client := NewByocBaseClient(context.Background(), client.GrpcClient{Loader: compose.Loader{}}, "project", dummyLister{})
		_, err := client.LoadProjectName(context.Background())
		if err != nil {
			t.Fatalf("LoadProjectName() failed: %v", err)
		}
	})

	t.Run("invalied COMPOSE_PROJECT_NAME", func(t *testing.T) {
		t.Setenv("COMPOSE_PROJECT_NAME", "as df")
		client := NewByocBaseClient(context.Background(), client.GrpcClient{Loader: compose.Loader{}}, "project", dummyLister{})
		_, err := client.LoadProjectName(context.Background())
		expected := loader.InvalidProjectNameErr("as df")
		if err.Error() != expected.Error() {
			t.Fatalf("LoadProjectName() failed: expected %v, got: %v", expected, err)
		}
	})
}
