package compose

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	composeTypes "github.com/compose-spec/compose-go/v2/types"
	"github.com/stretchr/testify/assert"
)

func TestFixup(t *testing.T) {
	testAllComposeFiles(t, func(t *testing.T, name string, path string) {
		loader := NewLoader(WithPath(path))
		proj, err := loader.LoadProject(t.Context())
		if strings.HasPrefix(name, "invalid-") {
			assert.Error(t, err, "Expected error for invalid compose file: %s", path)
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		err = FixupServices(t.Context(), &client.MockProvider{}, proj, UploadModeIgnore)
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
