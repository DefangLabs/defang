package compose

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/term"
	composeTypes "github.com/compose-spec/compose-go/v2/types"
	"go.yaml.in/yaml/v4"
)

// LLMExtension is the object form of x-defang-llm on a managed model service.
// The CD engine resolves Model through the deployment's recipe and injects the
// application-facing model name into each binding that it supplies, so the CLI
// never maps a request to a cloud-specific model.
type LLMExtension struct {
	// Model is the request exactly as written in Compose.
	Model    string         `yaml:"model" json:"model"`
	Bindings []ModelBinding `yaml:"bindings,omitempty" json:"bindings,omitempty"`
}

// ModelBinding records how one consumer service receives a managed model.
type ModelBinding struct {
	Consumer    string `yaml:"consumer" json:"consumer"`
	ModelVar    string `yaml:"model_var" json:"model_var"`
	EndpointVar string `yaml:"endpoint_var" json:"endpoint_var"`
	// Supplied lists the values Defang provides; a value the user set
	// explicitly is left out and must not be changed.
	Supplied []string `yaml:"supplied,omitempty" json:"supplied,omitempty"`
}

const (
	suppliedModel    = "model"
	suppliedEndpoint = "endpoint"
)

var errNoModel = errors.New("no model specified")

// ParseLLMExtension decodes an x-defang-llm value. It returns nil for the
// scalar form (`x-defang-llm: true`), which marks a user-managed LLM service.
func ParseLLMExtension(value any) (*LLMExtension, error) {
	switch v := value.(type) {
	case nil, bool:
		return nil, nil
	case LLMExtension:
		return &v, nil
	case *LLMExtension:
		return v, nil
	}
	// After a YAML round trip the extension is a generic map.
	data, err := yaml.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("invalid x-defang-llm: %w", err)
	}
	var ext LLMExtension
	if err := yaml.Unmarshal(data, &ext); err != nil {
		return nil, fmt.Errorf("invalid x-defang-llm: %w", err)
	}
	if ext.Model == "" {
		return nil, nil
	}
	return &ext, nil
}

// HasModelRequests reports whether the project asks the CD engine to resolve a
// managed model, which only engines implementing the model contract can do.
func HasModelRequests(project *composeTypes.Project) (bool, error) {
	for name, service := range project.Services {
		ext, err := ParseLLMExtension(service.Extensions["x-defang-llm"])
		if err != nil {
			return false, fmt.Errorf("service %q: %w", name, err)
		}
		if ext != nil {
			return true, nil
		}
	}
	return false, nil
}

// IsVirtualModelService reports whether svccfg only describes a model served
// by the cloud provider, so no container runs for it.
func IsVirtualModelService(svccfg *composeTypes.ServiceConfig) bool {
	_, llm := svccfg.Extensions["x-defang-llm"]
	return llm && svccfg.Image == "" && svccfg.Build == nil
}

func isModelProviderService(svccfg *composeTypes.ServiceConfig) bool {
	return svccfg.Provider != nil && svccfg.Provider.Type == "model" && svccfg.Image == "" && svccfg.Build == nil
}

func providerServiceModel(svccfg *composeTypes.ServiceConfig) string {
	if modelVals := svccfg.Provider.Options["model"]; len(modelVals) == 1 {
		return modelVals[0]
	}
	return ""
}

func warnUnsupportedModelOptions(model composeTypes.ModelConfig) {
	if model.ContextSize != 0 {
		term.Warnf("model %q: context_size is a Docker Model Runner parameter and is not supported for cloud deployments", model.Name)
	}
	if len(model.RuntimeFlags) > 0 {
		term.Warnf("model %q: runtime_flags is a Docker Model Runner parameter and is not supported for cloud deployments", model.Name)
	}
}

// FixupDirectModels turns every model request into a virtual model service for
// a provider that serves models from a public OpenAI-compatible endpoint. No
// gateway runs: consumers call endpointURL directly with the OPENAI_API_KEY
// config, and the CD engine injects the resolved model ID.
func FixupDirectModels(project *composeTypes.Project, endpointURL string) error {
	for name, service := range project.Services {
		if !isModelProviderService(&service) {
			continue
		}
		term.Warnf("service %q: 'provider: type: model' is deprecated; use a top-level 'models:' entry instead", name)
		if err := addDirectModelService(project, name, providerServiceModel(&service), endpointURL); err != nil {
			return fmt.Errorf("service %q: %w", name, err)
		}
	}
	for name, model := range project.Models {
		model.Name = name
		warnUnsupportedModelOptions(model)
		if err := addDirectModelService(project, name, model.Model, endpointURL); err != nil {
			return fmt.Errorf("model %q: %w", name, err)
		}
		delete(project.Models, name)
	}
	return nil
}

func addDirectModelService(project *composeTypes.Project, name, model, endpointURL string) error {
	if model == "" {
		return errNoModel
	}
	bindings := bindModelConsumers(project, name, modelWiring{endpoint: endpointURL, direct: true})
	project.Services[name] = composeTypes.ServiceConfig{
		Name:       name,
		Extensions: composeTypes.Extensions{"x-defang-llm": LLMExtension{Model: model, Bindings: bindings}},
	}
	return nil
}

type modelWiring struct {
	endpoint string
	// model is the value written to the model variable; nil leaves it for the
	// CD engine to inject.
	model *string
	// apiKey is written to OPENAI_API_KEY when unset; nil references Defang config.
	apiKey *string
	// direct endpoints run no gateway, so consumers neither join the private
	// model network nor depend on the (virtual) model service.
	direct bool
}

// bindModelConsumers wires every service that uses the model service svcName,
// either through depends_on (deprecated syntax) or a service-level models:
// entry, and returns one binding per wired variable pair. Local Docker sets
// [SERVICE]_URL and [SERVICE]_MODEL by default.
func bindModelConsumers(project *composeTypes.Project, svcName string, w modelWiring) []ModelBinding {
	envName := strings.ToUpper(svcName) // TODO: handle characters that are not allowed in env vars, like '-'
	defaultEndpointVar, defaultModelVar := envName+"_URL", envName+"_MODEL"

	var bindings []ModelBinding
	for _, name := range slices.Sorted(maps.Keys(project.Services)) {
		consumer := project.Services[name]
		changed := false

		if _, ok := consumer.DependsOn[svcName]; ok {
			bindings = append(bindings, bindModelConsumer(&consumer, name, defaultEndpointVar, defaultModelVar, w))
			if w.direct {
				delete(consumer.DependsOn, svcName)
			}
			changed = true
		}

		if modelDep, ok := consumer.Models[svcName]; ok {
			endpointVar, modelVar := defaultEndpointVar, defaultModelVar
			if modelDep != nil && modelDep.EndpointVariable != "" {
				endpointVar = modelDep.EndpointVariable
			}
			if modelDep != nil && modelDep.ModelVariable != "" {
				modelVar = modelDep.ModelVariable
			}
			binding := bindModelConsumer(&consumer, name, endpointVar, modelVar, w)
			if !slices.ContainsFunc(bindings, func(b ModelBinding) bool {
				return b.Consumer == name && b.EndpointVar == endpointVar && b.ModelVar == modelVar
			}) {
				bindings = append(bindings, binding)
			}
			if w.direct {
				delete(consumer.Models, svcName)
			} else if _, ok := consumer.DependsOn[svcName]; !ok {
				if consumer.DependsOn == nil {
					consumer.DependsOn = make(map[string]composeTypes.ServiceDependency)
				}
				consumer.DependsOn[svcName] = composeTypes.ServiceDependency{
					Condition: composeTypes.ServiceConditionStarted,
					Required:  true,
				}
			}
			changed = true
		}

		if changed {
			project.Services[name] = consumer
		}
	}
	return bindings
}

func bindModelConsumer(consumer *composeTypes.ServiceConfig, name, endpointVar, modelVar string, w modelWiring) ModelBinding {
	if consumer.Environment == nil {
		consumer.Environment = make(composeTypes.MappingWithEquals)
	}
	if !w.direct {
		if consumer.Networks == nil {
			consumer.Networks = make(map[string]*composeTypes.ServiceNetworkConfig)
		}
		consumer.Networks[modelProviderNetwork] = nil
	}
	binding := ModelBinding{Consumer: name, ModelVar: modelVar, EndpointVar: endpointVar}
	if _, ok := consumer.Environment[modelVar]; !ok {
		binding.Supplied = append(binding.Supplied, suppliedModel)
		if w.model != nil {
			consumer.Environment[modelVar] = w.model
		}
	}
	if _, ok := consumer.Environment[endpointVar]; !ok {
		binding.Supplied = append(binding.Supplied, suppliedEndpoint)
		consumer.Environment[endpointVar] = &w.endpoint
	}
	if _, ok := consumer.Environment["OPENAI_API_KEY"]; !ok {
		consumer.Environment["OPENAI_API_KEY"] = w.apiKey
	}
	return binding
}
