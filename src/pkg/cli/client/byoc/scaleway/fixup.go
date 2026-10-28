package scaleway

import (
	"context"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	composeTypes "github.com/compose-spec/compose-go/v2/types"
)

var _ compose.ServiceFixupper = (*ByocScaleway)(nil)

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
		if service.Provider != nil && service.Provider.Type == "model" && service.Image == "" && service.Build == nil {
			model := ""
			if values := service.Provider.Options["model"]; len(values) == 1 {
				model = values[0]
			}
			fixupRemoteModel(project, name, model)
			delete(project.Services, name)
		}
	}
	for name, model := range project.Models {
		fixupRemoteModel(project, name, model.Model)
		delete(project.Models, name)
	}
	return nil
}

func fixupRemoteModel(project *composeTypes.Project, name, model string) {
	envName := strings.ToUpper(name)
	wireModelDependents(project, name, "https://api.scaleway.ai/v1/", resolveModel(model), envName+"_URL", envName+"_MODEL")
}

func resolveModel(model string) string {
	switch model {
	case "chat-default":
		return "llama-3.3-70b-instruct"
	case "embedding-default":
		return "bge-multilingual-gemma2"
	default:
		return model
	}
}

// wireModelDependents injects URL, model, and API-key env vars into
// dependent services. Direct endpoints need no sidecar or private model network.
// A nil OPENAI_API_KEY marks a reference to Defang config.
func wireModelDependents(project *composeTypes.Project, svcName, urlVal, model, endpointEnvVar, modelEnvVar string) {
	for name, dependency := range project.Services {
		changed := false

		if _, ok := dependency.DependsOn[svcName]; ok {
			if dependency.Environment == nil {
				dependency.Environment = make(composeTypes.MappingWithEquals)
			}
			if _, ok := dependency.Environment[endpointEnvVar]; !ok {
				dependency.Environment[endpointEnvVar] = &urlVal
			}
			if _, ok := dependency.Environment[modelEnvVar]; !ok && model != "" {
				dependency.Environment[modelEnvVar] = &model
			}
			if _, ok := dependency.Environment["OPENAI_API_KEY"]; !ok {
				dependency.Environment["OPENAI_API_KEY"] = nil
			}
			// Remove dependency on the model service since it won't be deployed
			delete(dependency.DependsOn, svcName)
			changed = true
		}

		if modelDep, ok := dependency.Models[svcName]; ok {
			endpointVar := endpointEnvVar
			if modelDep != nil && modelDep.EndpointVariable != "" {
				endpointVar = modelDep.EndpointVariable
			}
			modelVar := modelEnvVar
			if modelDep != nil && modelDep.ModelVariable != "" {
				modelVar = modelDep.ModelVariable
			}
			if dependency.Environment == nil {
				dependency.Environment = make(composeTypes.MappingWithEquals)
			}
			if _, ok := dependency.Environment[endpointVar]; !ok {
				dependency.Environment[endpointVar] = &urlVal
			}
			if _, ok := dependency.Environment[modelVar]; !ok && model != "" {
				dependency.Environment[modelVar] = &model
			}
			if _, ok := dependency.Environment["OPENAI_API_KEY"]; !ok {
				dependency.Environment["OPENAI_API_KEY"] = nil
			}
			delete(dependency.Models, svcName)
			changed = true
		}

		if changed {
			project.Services[name] = dependency
		}
	}
}
