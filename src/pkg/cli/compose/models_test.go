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

const modelRequestsCompose = "testdata/model-requests/compose.yaml"

func loadModelRequests(t *testing.T) *Project {
	t.Helper()
	project, err := NewLoader(WithPath(modelRequestsCompose)).LoadProject(t.Context())
	require.NoError(t, err)
	return project
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
			assertModelRequests(t, reloaded, map[string]LLMExtension{
				"chat": {Model: "ai/chat-default", Bindings: []ModelBinding{
					{Consumer: "app", ModelVar: "CHAT_MODEL", EndpointVar: "CHAT_URL", Supplied: []string{"model"}},
					{Consumer: "worker", ModelVar: "WORKER_CHAT_MODEL", EndpointVar: "CHAT_URL", Supplied: []string{"model", "endpoint"}},
				}},
				"embed": {Model: "embedding-default", Bindings: []ModelBinding{
					{Consumer: "app", ModelVar: "EMBED_MODEL_NAME", EndpointVar: "EMBED_BASE_URL", Supplied: []string{"model", "endpoint"}},
				}},
				"exact": {Model: "us.amazon.nova-2-lite-v1:0", Bindings: []ModelBinding{
					{Consumer: "worker", ModelVar: "EXACT_MODEL", EndpointVar: "EXACT_URL", Supplied: []string{"model", "endpoint"}},
				}},
				"large": {Model: "chat-large", Bindings: []ModelBinding{
					{Consumer: "worker", ModelVar: "LARGE_MODEL", EndpointVar: "LARGE_URL", Supplied: []string{"endpoint"}},
				}},
			})

			for _, name := range []string{"chat", "embed", "exact", "large"} {
				gateway := reloaded.Services[name]
				assert.Empty(t, gateway.Command, "the CD engine owns the gateway command")
				assert.NotContains(t, gateway.Environment, "VERTEXAI_LOCATION", "the location depends on the resolved model")
				assert.Equal(t, "litellm/litellm:v1.92.0", gateway.Image)
			}
			app, worker := reloaded.Services["app"], reloaded.Services["worker"]
			assert.NotContains(t, app.Environment, "CHAT_MODEL", "the CD engine injects supplied model names")
			assert.NotContains(t, worker.Environment, "WORKER_CHAT_MODEL")
			assert.Equal(t, "https://proxy.example/v1/", *app.Environment["CHAT_URL"])
			assert.Equal(t, "my-pinned-name", *worker.Environment["LARGE_MODEL"])
		})
	}
}

func assertModelRequests(t *testing.T, project *Project, want map[string]LLMExtension) {
	t.Helper()
	got := map[string]LLMExtension{}
	for name, service := range project.Services {
		ext, err := ParseLLMExtension(service.Extensions["x-defang-llm"])
		require.NoError(t, err)
		if ext != nil {
			got[name] = *ext
		}
	}
	assert.Equal(t, want, got)
	hasRequests, err := HasModelRequests(project)
	require.NoError(t, err)
	assert.Equal(t, len(want) > 0, hasRequests)
}

func TestFixupModelRequestErrors(t *testing.T) {
	tests := []struct {
		name     string
		provider client.ProviderID
		compose  string
		wantErr  string
	}{
		{
			name:     "DigitalOcean has no managed models",
			provider: client.ProviderDO,
			compose:  "models:\n  llm:\n    model: chat-default\n",
			wantErr:  `model "llm": managed models are not supported on DigitalOcean`,
		},
		{
			name:     "deprecated syntax without a model",
			provider: client.ProviderAWS,
			compose:  "  llm:\n    provider:\n      type: model\n",
			wantErr:  `service "llm": no model specified`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			project, err := LoadFromContent(t.Context(), []byte("name: demo\nservices:\n  app:\n    image: app\n"+tt.compose), "")
			require.NoError(t, err)
			err = FixupServices(t.Context(), accountProvider{info: client.AccountInfo{Provider: tt.provider}}, project, UploadModeIgnore)
			assert.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestFixupDirectModels(t *testing.T) {
	project := loadModelRequests(t)
	require.NoError(t, FixupDirectModels(project, "https://inference.example/v1/"))

	assert.Empty(t, project.Models)
	assertModelRequests(t, project, map[string]LLMExtension{
		"chat": {Model: "ai/chat-default", Bindings: []ModelBinding{
			{Consumer: "app", ModelVar: "CHAT_MODEL", EndpointVar: "CHAT_URL", Supplied: []string{"model"}},
			{Consumer: "worker", ModelVar: "WORKER_CHAT_MODEL", EndpointVar: "CHAT_URL", Supplied: []string{"model", "endpoint"}},
		}},
		"embed": {Model: "embedding-default", Bindings: []ModelBinding{
			{Consumer: "app", ModelVar: "EMBED_MODEL_NAME", EndpointVar: "EMBED_BASE_URL", Supplied: []string{"model", "endpoint"}},
		}},
		"exact": {Model: "us.amazon.nova-2-lite-v1:0", Bindings: []ModelBinding{
			{Consumer: "worker", ModelVar: "EXACT_MODEL", EndpointVar: "EXACT_URL", Supplied: []string{"model", "endpoint"}},
		}},
		"large": {Model: "chat-large", Bindings: []ModelBinding{
			{Consumer: "worker", ModelVar: "LARGE_MODEL", EndpointVar: "LARGE_URL", Supplied: []string{"endpoint"}},
		}},
	})
	for _, name := range []string{"chat", "embed", "exact", "large"} {
		assert.Equal(t, composeTypes.ServiceConfig{Name: name, Extensions: project.Services[name].Extensions}, project.Services[name],
			"a direct model service is virtual: no container, gateway or network")
	}
	worker := project.Services["worker"]
	assert.Empty(t, worker.DependsOn)
	assert.Empty(t, worker.Models)
	assert.NotContains(t, worker.Networks, modelProviderNetwork)
	assert.Equal(t, "https://inference.example/v1/", *worker.Environment["CHAT_URL"])
	assert.Contains(t, worker.Environment, "OPENAI_API_KEY")
	assert.Nil(t, worker.Environment["OPENAI_API_KEY"], "the API key is a config reference")
	assert.NotContains(t, worker.Environment, "WORKER_CHAT_MODEL")

	t.Run("missing model", func(t *testing.T) {
		project := &Project{Services: composeTypes.Services{}, Models: map[string]composeTypes.ModelConfig{"llm": {}}}
		assert.ErrorContains(t, FixupDirectModels(project, "https://inference.example/v1/"), `model "llm": no model specified`)
	})
}

func TestParseLLMExtension(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  *LLMExtension
	}{
		{name: "scalar marks a user-managed gateway", value: true},
		{name: "empty object from an old CLI", value: map[string]any{}},
		{name: "decoded YAML", value: map[string]any{
			"model":    "chat-default",
			"bindings": []any{map[string]any{"consumer": "app", "model_var": "M", "endpoint_var": "U", "supplied": []any{"model"}}},
		}, want: &LLMExtension{Model: "chat-default", Bindings: []ModelBinding{{Consumer: "app", ModelVar: "M", EndpointVar: "U", Supplied: []string{"model"}}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseLLMExtension(tt.value)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
	_, err := ParseLLMExtension(map[string]any{"model": []any{"not", "a", "string"}})
	assert.Error(t, err)
}
