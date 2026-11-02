package compose

import (
	"context"
	"strings"
	"testing"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	composeTypes "github.com/compose-spec/compose-go/v2/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type accountProvider struct {
	client.MockProvider
	info client.AccountInfo
}

func (p accountProvider) AccountInfo(context.Context) (*client.AccountInfo, error) {
	return &p.info, nil
}

type directProvider struct {
	client.MockProvider
}

func (directProvider) DirectModelEndpoint() string { return "https://inference.example/v1/" }

const modelRequestsCompose = "testdata/model-requests/compose.yaml"

func loadModelRequests(t *testing.T) *Project {
	t.Helper()
	project, err := NewLoader(WithPath(modelRequestsCompose)).LoadProject(t.Context())
	require.NoError(t, err)
	return project
}

func binding(endpointVar, modelVar string) *composeTypes.ServiceModelConfig {
	return &composeTypes.ServiceModelConfig{EndpointVariable: endpointVar, ModelVariable: modelVar}
}

// Requests stay in models:, and every consumer names both of its variables.
var wantModelRequests = map[string]string{
	"chat":  "ai/chat-default",
	"embed": "embedding-default",
	"exact": "us.amazon.nova-2-lite-v1:0",
	"large": "chat-large",
}

var wantBindings = map[string]map[string]*composeTypes.ServiceModelConfig{
	"app": {
		"chat":  binding("CHAT_URL", "CHAT_MODEL"),
		"embed": binding("EMBED_BASE_URL", "EMBED_MODEL_NAME"),
	},
	"worker": {
		"chat":  binding("CHAT_URL", "WORKER_CHAT_MODEL"),
		"exact": binding("EXACT_URL", "EXACT_MODEL"),
		"large": binding("LARGE_URL", "LARGE_MODEL"),
	},
}

func assertModelPayload(t *testing.T, project *Project) {
	t.Helper()
	requests := map[string]string{}
	for name, model := range project.Models {
		requests[name] = model.Model
	}
	assert.Equal(t, wantModelRequests, requests)
	for consumer, want := range wantBindings {
		assert.Equal(t, want, project.Services[consumer].Models, consumer)
	}
}

// The payload is what the CD engine receives, so it is compared as a whole.
func TestFixupModelRequestPayloads(t *testing.T) {
	for _, info := range []client.AccountInfo{
		{Provider: client.ProviderAWS, Region: "us-west-2", AccountID: "123456789012"},
		{Provider: client.ProviderGCP, Region: "us-central1", AccountID: "my-project"},
		{Provider: client.ProviderAzure, Region: "eastus", AccountID: "my-subscription"},
	} {
		t.Run(info.Provider.String(), func(t *testing.T) {
			project := loadModelRequests(t)
			require.NoError(t, FixupServices(t.Context(), accountProvider{info: info}, project, UploadModeIgnore))
			payload, err := MarshalYAML(project)
			require.NoError(t, err)
			require.NoError(t, pkg.Compare(payload, modelRequestsCompose+"."+strings.ToLower(info.Provider.String())+".golden"))

			reloaded, err := LoadFromContent(t.Context(), payload, "")
			require.NoError(t, err)
			assertModelPayload(t, reloaded)
			for name := range wantModelRequests {
				gateway := reloaded.Services[name]
				assert.Equal(t, "litellm/litellm:v1.92.0", gateway.Image, name)
				assert.Equal(t, true, gateway.Extensions["x-defang-llm"], name)
				assert.Empty(t, gateway.Command, "%s: the CD engine owns the gateway routing", name)
				assert.NotContains(t, gateway.Environment, "VERTEXAI_LOCATION", "%s: the location depends on the resolved model", name)
			}
			assert.Equal(t, "DEBUG", *reloaded.Services["large"].Environment["LITELLM_LOG"])

			app, worker := reloaded.Services["app"], reloaded.Services["worker"]
			assert.Equal(t, "chat-default", *app.Environment["CHAT_MODEL"], "the ai/ spelling of a built-in alias is normalized")
			assert.Equal(t, "https://proxy.example/v1/", *app.Environment["CHAT_URL"], "explicit endpoint stays authoritative")
			assert.Equal(t, "embedding-default", *app.Environment["EMBED_MODEL_NAME"])
			assert.Equal(t, "chat-default", *worker.Environment["WORKER_CHAT_MODEL"])
			assert.Equal(t, "us.amazon.nova-2-lite-v1:0", *worker.Environment["EXACT_MODEL"])
			assert.Equal(t, "my-pinned-name", *worker.Environment["LARGE_MODEL"], "explicit model stays authoritative")
		})
	}
}

func TestFixupDirectModels(t *testing.T) {
	project := loadModelRequests(t)
	require.NoError(t, FixupServices(t.Context(), directProvider{}, project, UploadModeIgnore))

	assertModelPayload(t, project)
	for name := range wantModelRequests {
		assert.NotContains(t, project.Services, name, "a provider-served model has no service")
	}
	assert.NotContains(t, project.Networks, modelProviderNetwork)
	for _, name := range []string{"app", "worker"} {
		consumer := project.Services[name]
		assert.Empty(t, consumer.DependsOn, name)
		assert.NotContains(t, consumer.Networks, modelProviderNetwork, name)
		assert.Contains(t, consumer.Environment, "OPENAI_API_KEY", name)
		assert.Nil(t, consumer.Environment["OPENAI_API_KEY"], "%s: the API key is a config reference", name)
	}
	app, worker := project.Services["app"], project.Services["worker"]
	assert.Equal(t, "https://inference.example/v1/", *app.Environment["EMBED_BASE_URL"])
	assert.Equal(t, "https://proxy.example/v1/", *app.Environment["CHAT_URL"])
	for _, modelVar := range []string{"CHAT_MODEL", "EMBED_MODEL_NAME"} {
		assert.NotContains(t, app.Environment, modelVar, "the CD engine writes the resolved model ID")
	}
	assert.Equal(t, "my-pinned-name", *worker.Environment["LARGE_MODEL"])

	payload, err := MarshalYAML(project)
	require.NoError(t, err)
	_, err = LoadFromContent(t.Context(), payload, "")
	require.NoError(t, err, "the payload must not reference a missing model or service")
}

func TestFixupModelRequestErrors(t *testing.T) {
	tests := []struct {
		name     string
		provider client.Provider
		compose  string
		wantErr  string
	}{
		{
			name:     "DigitalOcean has no managed models",
			provider: accountProvider{info: client.AccountInfo{Provider: client.ProviderDO}},
			compose:  "models:\n  llm:\n    model: chat-default\n",
			wantErr:  "managed models are not supported on DigitalOcean",
		},
		{
			name:     "deprecated syntax without a model",
			provider: accountProvider{info: client.AccountInfo{Provider: client.ProviderAWS}},
			compose:  "  llm:\n    provider:\n      type: model\n",
			wantErr:  `service "llm": no model specified`,
		},
		{
			name:     "top-level model without a model",
			provider: directProvider{},
			compose:  "models:\n  llm:\n    model: \"\"\n",
			wantErr:  `model "llm": no model specified`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			project, err := LoadFromContent(t.Context(), []byte("name: demo\nservices:\n  app:\n    image: app\n"+tt.compose), "")
			require.NoError(t, err)
			assert.ErrorContains(t, FixupServices(t.Context(), tt.provider, project, UploadModeIgnore), tt.wantErr)
		})
	}
}

func TestApplicationModelName(t *testing.T) {
	for model, want := range map[string]string{
		"ai/chat-default":      "chat-default",
		"ai/chat-large":        "chat-large",
		"ai/embedding-default": "embedding-default",
		"ai/smollm2":           "ai/smollm2",
		"chat-default":         "chat-default",
		"bedrock/ai/model":     "bedrock/ai/model",
	} {
		assert.Equal(t, want, applicationModelName(model), model)
	}
}
