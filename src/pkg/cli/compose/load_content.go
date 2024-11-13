package compose

import (
	"context"

	"github.com/compose-spec/compose-go/v2/loader"
	composeTypes "github.com/compose-spec/compose-go/v2/types"
)

func LoadFromContent(ctx context.Context, content []byte, nameFallback string) (*Project, error) {
	return loader.LoadWithContext(ctx, composeTypes.ConfigDetails{ConfigFiles: []composeTypes.ConfigFile{{Content: content}}}, func(o *loader.Options) {
		o.SetProjectName(nameFallback, false)
		o.SkipConsistencyCheck = true // this matches the WithConsistency(false) option from the loader
		o.SkipInterpolation = true
		o.SkipResolveEnvironment = true
		o.SkipInclude = true
	})
}
