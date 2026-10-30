package scaleway

import (
	"maps"
	"net/http"
	"slices"
	"testing"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	composeTypes "github.com/compose-spec/compose-go/v2/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fixupTestProvider(t *testing.T) *ByocScaleway {
	t.Helper()
	provider := newTestProvider("prod")
	provider.client.HTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		assert.Equal(t, "/secret-manager/v1beta1/regions/fr-par/secrets", req.URL.Path)
		return jsonResponse(`{"secrets":[]}`), nil
	})}
	return provider
}

// The same application as the gateway clouds' payload tests in the compose package.
const modelRequestsCompose = "../../../compose/testdata/model-requests/compose.yaml"

func TestScalewayFixupModelRequestPayload(t *testing.T) {
	project, err := compose.NewLoader(compose.WithPath(modelRequestsCompose)).LoadProject(t.Context())
	require.NoError(t, err)
	provider := fixupTestProvider(t)
	require.NoError(t, compose.FixupServices(t.Context(), provider, project, compose.UploadModeIgnore))
	payload, err := compose.MarshalYAML(project)
	require.NoError(t, err)
	require.NoError(t, pkg.Compare(payload, "testdata/model-requests.scaleway.golden"))

	reloaded, err := compose.LoadFromContent(t.Context(), payload, "")
	require.NoError(t, err, "the deploy payload must have no orphan model references")
	assert.Empty(t, reloaded.Models)
	assert.NotContains(t, reloaded.Networks, "model_provider_private")
	for _, name := range []string{"chat", "embed", "exact", "large"} {
		service := reloaded.Services[name]
		assert.Empty(t, service.Image, "Generative APIs serve the model; no container runs")
		ext, err := compose.ParseLLMExtension(service.Extensions["x-defang-llm"])
		require.NoError(t, err)
		require.NotNil(t, ext)
	}
	app := reloaded.Services["app"]
	assert.Equal(t, "https://proxy.example/v1/", *app.Environment["CHAT_URL"], "explicit endpoint stays authoritative")
	assert.Equal(t, generativeAPIsURL, *app.Environment["EMBED_BASE_URL"])
	assert.NotContains(t, app.Environment, "EMBED_MODEL_NAME", "the CD engine injects the resolved model ID")
	assert.Contains(t, app.Environment, "OPENAI_API_KEY")
	assert.Nil(t, app.Environment["OPENAI_API_KEY"], "the API key is a Secret Manager reference")
	assert.Empty(t, app.DependsOn)
	assert.Empty(t, app.Models)
	assert.Equal(t, []string{"app", "worker"}, slices.Sorted(maps.Keys(containerServices(reloaded.Services))))

	require.NoError(t, compose.FixupServices(t.Context(), provider, project, compose.UploadModeIgnore))
	again, err := compose.MarshalYAML(project)
	require.NoError(t, err)
	assert.Equal(t, string(payload), string(again), "fixing up again must not change the payload")
}

func TestScalewayFixupPreservesOverridesAndRequiresOnlyManagedRedisConfig(t *testing.T) {
	password, endpoint, key := "existing-password", "https://custom.example/v1/", "existing-key" //nolint:gosec // Fake override values.
	project := &composeTypes.Project{
		Name: "demo",
		Services: composeTypes.Services{
			"cache": {Name: "cache", Image: "redis:8", Extensions: composeTypes.Extensions{"x-defang-redis": true}},
			"configured": {Name: "configured", Image: "redis:8", Extensions: composeTypes.Extensions{"x-defang-redis": true},
				Environment: composeTypes.MappingWithEquals{"REDIS_PASSWORD": &password}},
			"self": {Name: "self", Image: "redis:8"},
			"app": {Name: "app", Image: "python:3.12-alpine",
				Environment: composeTypes.MappingWithEquals{"LLM_URL": &endpoint, "OPENAI_API_KEY": &key},
				Models:      map[string]*composeTypes.ServiceModelConfig{"llm": nil}},
		},
		Models: map[string]composeTypes.ModelConfig{"llm": {Model: "chat-default"}},
	}
	require.NoError(t, compose.FixupServices(t.Context(), fixupTestProvider(t), project, compose.UploadModeIgnore))
	assert.Contains(t, project.Services["cache"].Environment, "REDIS_PASSWORD")
	assert.Nil(t, project.Services["cache"].Environment["REDIS_PASSWORD"])
	assert.Equal(t, password, *project.Services["configured"].Environment["REDIS_PASSWORD"])
	assert.NotContains(t, project.Services["self"].Environment, "REDIS_PASSWORD")
	assert.Equal(t, endpoint, *project.Services["app"].Environment["LLM_URL"])
	assert.Equal(t, key, *project.Services["app"].Environment["OPENAI_API_KEY"])
}
