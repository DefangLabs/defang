package scaleway

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	composeTypes "github.com/compose-spec/compose-go/v2/types"
)

var (
	_ compose.ServiceFixupper     = (*ByocScaleway)(nil)
	_ compose.DirectModelProvider = (*ByocScaleway)(nil)
)

// generativeAPIsURL is the OpenAI-compatible endpoint of Scaleway Generative APIs.
const generativeAPIsURL = "https://api.scaleway.ai/v1/"

// DirectModelEndpoint implements compose.DirectModelProvider: consumers call
// Generative APIs directly, so no model gateway is deployed.
func (*ByocScaleway) DirectModelEndpoint() string {
	return generativeAPIsURL
}

// FixupServices applies Scaleway requirements before the shared Compose fixups.
func (*ByocScaleway) FixupServices(_ context.Context, project *composeTypes.Project) error {
	for name, service := range project.Services {
		if _, managed := service.Extensions["x-defang-redis"]; managed {
			if service.Environment == nil {
				service.Environment = make(composeTypes.MappingWithEquals)
			}
			if _, exists := service.Environment["REDIS_PASSWORD"]; !exists {
				service.Environment["REDIS_PASSWORD"] = nil
			}
			project.Services[name] = service
		}
	}
	return nil
}
