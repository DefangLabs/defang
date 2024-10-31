package compose

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	composeTypes "github.com/compose-spec/compose-go/v2/types"
)

func TestFixup(t *testing.T) {
	testRunCompose(t, func(t *testing.T, path string) {
		t.Helper()
		loader := NewLoaderWithPath(path)
		proj, err := loader.LoadProject(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		err = FixupServices(context.Background(), client.MockProvider{}, proj, UploadModeIgnore)
		if err != nil {
			t.Fatal(err)
		}

		var services []composeTypes.ServiceConfig
		for _, svc := range proj.Services {
			services = append(services, svc)
		}

		// The order of the services is not guaranteed, so we sort the services before comparing
		slices.SortFunc(services, func(i, j composeTypes.ServiceConfig) int { return strings.Compare(i.Name, j.Name) })

		// Convert the protobuf services to pretty JSON for comparison (YAML would include all the zero values)
		actual, err := json.MarshalIndent(services, "", "  ")
		if err != nil {
			t.Fatal(err)
		}

		if err := compare(actual, path+".fixup"); err != nil {
			t.Error(err)
		}
	})
}
