package compose

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	composeTypes "github.com/compose-spec/compose-go/v2/types"
)

func TestFixup(t *testing.T) {
	testRunCompose(t, func(t *testing.T, path string) {
		loader := NewLoader(WithPath(path))
		proj, err := loader.LoadProject(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		err = FixupServices(context.Background(), client.MockProvider{}, proj, UploadModeIgnore)
		if err != nil {
			t.Fatal(err)
		}

		services := map[string]composeTypes.ServiceConfig{}
		for _, svc := range proj.Services {
			services[svc.Name] = svc
		}

		// Convert the protobuf services to pretty JSON for comparison (YAML would include all the zero values)
		actual, err := json.MarshalIndent(services, "", "  ")
		if err != nil {
			t.Fatal(err)
		}

		if err := pkg.Compare(actual, path+".fixup"); err != nil {
			t.Error(err)
		}
	})
}
