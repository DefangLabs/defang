package cli

import (
	"context"
	"iter"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/modes"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

// previewFabric captures the PreviewRequest sent by GeneratePreview and inherits
// GetRecipe from MockFabricClient (which rejects an empty recipe name).
type previewFabric struct {
	client.MockFabricClient
	previewReq *defangv1.PreviewRequest
}

func (f *previewFabric) Preview(ctx context.Context, req *defangv1.PreviewRequest) (*defangv1.PreviewResponse, error) {
	f.previewReq = req
	return &defangv1.PreviewResponse{Etag: "preview-etag"}, nil
}

// previewLogProvider returns an empty log stream so GeneratePreview's tail loop
// finishes immediately instead of blocking.
type previewLogProvider struct {
	*mockDeployProvider
}

func (previewLogProvider) QueryLogs(ctx context.Context, req *defangv1.TailRequest) (iter.Seq2[*defangv1.TailResponse, error], error) {
	return func(yield func(*defangv1.TailResponse, error) bool) {}, nil
}

// TestGeneratePreview verifies the recipe/mode sent to the fabric: an unspecified
// recipe is left for the fabric to default (rather than sending an empty recipe
// name to GetRecipe, which the fabric rejects), while a specified recipe is used.
func TestGeneratePreview(t *testing.T) {
	project := &compose.Project{
		Name: "test-project",
		Services: compose.Services{
			"service1": compose.ServiceConfig{
				Name:       "service1",
				Image:      "test-image",
				DomainName: "test-domain",
			},
		},
	}

	tests := []struct {
		name       string
		recipe     modes.Recipe
		wantRecipe string // recipe name in the PreviewRequest; "" means unset (fabric defaults)
		wantMode   defangv1.DeploymentMode
	}{
		{
			name:       "unspecified recipe defers to the fabric",
			recipe:     modes.RecipeUnspecified,
			wantRecipe: "",
			wantMode:   defangv1.DeploymentMode_MODE_UNSPECIFIED,
		},
		{
			name:       "specified recipe is used as-is",
			recipe:     modes.RecipeAffordable,
			wantRecipe: "AFFORDABLE",
			wantMode:   defangv1.DeploymentMode_DEVELOPMENT,
		},
		{
			name:       "custom recipe is used as-is",
			recipe:     "FOO",
			wantRecipe: "FOO",
			wantMode:   defangv1.DeploymentMode_MODE_UNSPECIFIED, // custom recipes have no corresponding built-in mode
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fabric := &previewFabric{}
			provider := previewLogProvider{&mockDeployProvider{}}

			_, err := GeneratePreview(t.Context(), project, fabric, provider, client.ProviderAuto, tt.recipe, "us-test-1")
			if err != nil {
				t.Fatalf("GeneratePreview() failed: %v", err)
			}
			if fabric.previewReq == nil {
				t.Fatal("fabric.Preview was not called")
			}
			if got := fabric.previewReq.GetRecipe().GetName(); got != tt.wantRecipe {
				t.Errorf("PreviewRequest recipe = %q, want %q", got, tt.wantRecipe)
			}
			if got := fabric.previewReq.GetMode(); got != tt.wantMode {
				t.Errorf("PreviewRequest mode = %v, want %v", got, tt.wantMode)
			}
		})
	}
}
