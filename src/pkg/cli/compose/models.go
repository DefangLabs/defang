package compose

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	composeTypes "github.com/compose-spec/compose-go/v2/types"
)

// DirectModelProvider is implemented by providers that serve managed models
// from a public OpenAI-compatible endpoint, so no model gateway is deployed.
type DirectModelProvider interface {
	DirectModelEndpoint() string
}

var errNoModel = errors.New("no model specified")

type modelMode int

const (
	// modelModeLiteral routes the gateway to the model name as written; the
	// Playground has no BYOC CD engine that could resolve it.
	modelModeLiteral modelMode = iota
	// modelModeGateway leaves the gateway's routing to the CD engine, which
	// resolves models.<name>.model through the deployment's recipe.
	modelModeGateway
	// modelModeDirect deploys nothing for a model; the CD engine writes the
	// resolved model ID into each consumer variable that is unset.
	modelModeDirect
)

// fixupModels turns model requests into the payload the CD engine expects.
// On BYOC clouds the request stays in the top-level models: section and every
// consumer's services.*.models entry names both of its variables, so the CD
// engine never derives a variable name itself.
func fixupModels(provider client.Provider, project *composeTypes.Project, info *client.AccountInfo) error {
	mode := modelModeLiteral
	var directEndpoint string
	switch info.Provider {
	case client.ProviderAWS, client.ProviderGCP, client.ProviderAzure:
		mode = modelModeGateway
	case client.ProviderDO:
		if hasModels(project) {
			return errors.New("managed models are not supported on DigitalOcean; deploy your own model gateway as a regular service instead")
		}
		return nil
	default:
		if direct, ok := provider.(DirectModelProvider); ok {
			mode, directEndpoint = modelModeDirect, direct.DirectModelEndpoint()
		}
	}

	// The deprecated syntax becomes a top-level models: entry, so the CD engine
	// only ever sees one form.
	gatewayBase := map[string]composeTypes.ServiceConfig{}
	for _, name := range slices.Sorted(maps.Keys(project.Services)) {
		svccfg := project.Services[name]
		if !isModelProviderService(&svccfg) {
			continue
		}
		term.Warnf("service %q: 'provider: type: model' is deprecated; use a top-level 'models:' entry instead", name)
		model := providerServiceModel(&svccfg)
		if mode == modelModeLiteral {
			project.Services[name] = makeLiteralGateway(svccfg, project, model)
			continue
		}
		if model == "" {
			return fmt.Errorf("service %q: %w", name, errNoModel)
		}
		if _, exists := project.Models[name]; exists {
			return fmt.Errorf("service %q: a top-level model has the same name", name)
		}
		if project.Models == nil {
			project.Models = make(map[string]composeTypes.ModelConfig)
		}
		project.Models[name] = composeTypes.ModelConfig{Model: model}
		delete(project.Services, name)
		gatewayBase[name] = svccfg // keeps the user's settings for the gateway
	}

	for _, name := range slices.Sorted(maps.Keys(project.Models)) {
		model := project.Models[name]
		model.Name = name
		warnUnsupportedModelOptions(model)
		if mode != modelModeLiteral && model.Model == "" {
			return fmt.Errorf("model %q: %w", name, errNoModel)
		}
		switch mode {
		case modelModeLiteral:
			svccfg := makeLiteralGateway(composeTypes.ServiceConfig{Name: name, Extensions: maps.Clone(model.Extensions)}, project, model.Model)
			if _, ok := svccfg.Extensions["x-defang-llm"]; !ok {
				svccfg.Extensions["x-defang-llm"] = true
			}
			project.Services[name] = svccfg
		case modelModeGateway:
			svccfg, ok := gatewayBase[name]
			if !ok {
				svccfg = composeTypes.ServiceConfig{Name: name, Extensions: maps.Clone(model.Extensions)}
			}
			masterKey := configureAccessGateway(&svccfg, project)
			switch info.Provider {
			case client.ProviderAWS:
				if info.Region != "" {
					svccfg.Environment["AWS_REGION"] = &info.Region
				}
			case client.ProviderGCP:
				// The Vertex location depends on the resolved model, so the CD engine sets VERTEXAI_LOCATION.
				if info.AccountID != "" {
					svccfg.Environment["VERTEXAI_PROJECT"] = &info.AccountID
				}
			}
			if svccfg.Extensions == nil {
				svccfg.Extensions = make(composeTypes.Extensions)
			}
			if _, ok := svccfg.Extensions["x-defang-llm"]; !ok {
				svccfg.Extensions["x-defang-llm"] = true
			}
			project.Services[name] = svccfg
			appModel := applicationModelName(model.Model)
			wireModelConsumers(project, name, modelWiring{endpoint: gatewayURL(name), model: &appModel, apiKey: masterKey, explicit: true})
		case modelModeDirect:
			wireModelConsumers(project, name, modelWiring{endpoint: directEndpoint, direct: true, explicit: true})
		}
	}
	return nil
}

func hasModels(project *composeTypes.Project) bool {
	if len(project.Models) > 0 {
		return true
	}
	for _, svccfg := range project.Services {
		if isModelProviderService(&svccfg) {
			return true
		}
	}
	return false
}

// applicationModelName is the model name a consumer sends to the gateway. The
// CD engine serves the same name, so both sides normalize exactly the
// Docker-style spellings of the built-in aliases and nothing else.
func applicationModelName(model string) string {
	switch model {
	case "ai/chat-default", "ai/chat-large", "ai/embedding-default":
		return strings.TrimPrefix(model, "ai/")
	}
	return model
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

func makeLiteralGateway(svccfg composeTypes.ServiceConfig, project *composeTypes.Project, model string) composeTypes.ServiceConfig {
	if svccfg.Extensions == nil {
		svccfg.Extensions = make(composeTypes.Extensions)
	}
	masterKey := configureAccessGateway(&svccfg, project)
	svccfg.Command = []string{"--drop_params", "--model", model, "--alias", model}
	var modelVal *string
	if model != "" {
		modelVal = &model
	}
	wireModelConsumers(project, svccfg.Name, modelWiring{endpoint: gatewayURL(svccfg.Name), model: modelVal, apiKey: masterKey})
	return svccfg
}

type modelWiring struct {
	endpoint string
	// model is written to unset model variables; nil leaves them to the CD engine.
	model *string
	// apiKey is written to an unset OPENAI_API_KEY; nil references Defang config.
	apiKey *string
	// direct endpoints run no gateway: consumers neither join the private model
	// network nor depend on the model.
	direct bool
	// explicit records both variable names in the consumer's models: entry.
	explicit bool
}

// wireModelConsumers wires every service that uses the model svcName, either
// through depends_on (deprecated syntax) or a services.*.models entry. Local
// Docker sets [SERVICE]_URL and [SERVICE]_MODEL by default.
func wireModelConsumers(project *composeTypes.Project, svcName string, w modelWiring) {
	envName := strings.ToUpper(svcName) // TODO: handle characters that are not allowed in env vars, like '-'
	defaultEndpointVar, defaultModelVar := envName+"_URL", envName+"_MODEL"

	for name, consumer := range project.Services {
		_, dependent := consumer.DependsOn[svcName]
		modelDep, modelConsumer := consumer.Models[svcName]
		if !dependent && !modelConsumer {
			continue
		}

		if dependent {
			wireModelVariables(&consumer, defaultEndpointVar, defaultModelVar, w)
		}

		endpointVar, modelVar := defaultEndpointVar, defaultModelVar
		if modelDep != nil && modelDep.EndpointVariable != "" {
			endpointVar = modelDep.EndpointVariable
		}
		if modelDep != nil && modelDep.ModelVariable != "" {
			modelVar = modelDep.ModelVariable
		}
		if modelConsumer {
			wireModelVariables(&consumer, endpointVar, modelVar, w)
		}
		if w.explicit {
			if consumer.Models == nil {
				consumer.Models = make(map[string]*composeTypes.ServiceModelConfig)
			}
			binding := composeTypes.ServiceModelConfig{EndpointVariable: endpointVar, ModelVariable: modelVar}
			if modelDep != nil {
				binding.Extensions = modelDep.Extensions
			}
			consumer.Models[svcName] = &binding
		}

		if w.direct {
			delete(consumer.DependsOn, svcName)
		} else if _, ok := consumer.DependsOn[svcName]; !ok {
			if consumer.DependsOn == nil {
				consumer.DependsOn = make(map[string]composeTypes.ServiceDependency)
			}
			consumer.DependsOn[svcName] = composeTypes.ServiceDependency{
				Condition: composeTypes.ServiceConditionStarted,
				Required:  true,
			}
		}
		project.Services[name] = consumer
	}
}

func wireModelVariables(consumer *composeTypes.ServiceConfig, endpointVar, modelVar string, w modelWiring) {
	if consumer.Environment == nil {
		consumer.Environment = make(composeTypes.MappingWithEquals)
	}
	if !w.direct {
		if consumer.Networks == nil {
			consumer.Networks = make(map[string]*composeTypes.ServiceNetworkConfig)
		}
		consumer.Networks[modelProviderNetwork] = nil
	}
	if _, ok := consumer.Environment[endpointVar]; !ok {
		consumer.Environment[endpointVar] = &w.endpoint
	}
	if _, ok := consumer.Environment[modelVar]; !ok && w.model != nil {
		consumer.Environment[modelVar] = w.model
	}
	if _, ok := consumer.Environment["OPENAI_API_KEY"]; !ok {
		consumer.Environment["OPENAI_API_KEY"] = w.apiKey
	}
}
