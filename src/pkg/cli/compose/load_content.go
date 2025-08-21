package compose

import (
	"context"

	"github.com/compose-spec/compose-go/v2/loader"
	composeTypes "github.com/compose-spec/compose-go/v2/types"
)

func loadFromContent(ctx context.Context, content []byte, nameFallback string, skipInterpolation bool) (*Project, error) {
	return loader.LoadWithContext(ctx, composeTypes.ConfigDetails{ConfigFiles: []composeTypes.ConfigFile{{Content: content}}}, func(o *loader.Options) {
		o.SetProjectName(nameFallback, false)
		o.SkipConsistencyCheck = true
		o.SkipInterpolation = skipInterpolation
		o.SkipResolveEnvironment = true
		o.SkipInclude = true
	})
}

func LoadFromContent(ctx context.Context, content []byte, nameFallback string) (*Project, error) {
	return loadFromContent(ctx, content, nameFallback, true)
}

func LoadFromContentWithInterpolation(ctx context.Context, content []byte, nameFallback string) (*Project, error) {
	return loadFromContent(ctx, content, nameFallback, false)
}
