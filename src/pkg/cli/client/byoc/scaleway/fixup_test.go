package scaleway

import (
	"fmt"
	"net/http"
	"testing"

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

func TestScalewayFixupConsumesRemoteModelsBeforeSharedFixups(t *testing.T) {
	for _, model := range []struct{ input, resolved string }{
		{"chat-default", "qwen3-235b-a22b-instruct-2507"},
		{"embedding-default", "bge-multilingual-gemma2"},
		{"custom/model", "custom/model"},
	} {
		for _, topLevel := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/top-level=%v", model.input, topLevel), func(t *testing.T) {
				content := `name: modeldemo
services:
  app:
    image: python:3.12-alpine
`
				if topLevel {
					content += fmt.Sprintf(`    models:
      llm:
        endpoint_var: MODEL_URL
        model_var: MODEL_NAME
models:
  llm:
    model: %s
`, model.input)
				} else {
					content += fmt.Sprintf(`    depends_on:
      - llm
  llm:
    provider:
      type: model
      options:
        model: %s
`, model.input)
				}
				project, err := compose.LoadFromContent(t.Context(), []byte(content), "")
				require.NoError(t, err)
				provider := fixupTestProvider(t)
				require.NoError(t, compose.FixupServices(t.Context(), provider, project, compose.UploadModeIgnore))
				require.Len(t, project.Services, 1)
				assert.Empty(t, project.Models)
				app := project.Services["app"]
				endpointKey, modelKey := "LLM_URL", "LLM_MODEL"
				if topLevel {
					endpointKey, modelKey = "MODEL_URL", "MODEL_NAME"
				}
				require.NotNil(t, app.Environment[endpointKey])
				assert.Equal(t, "https://api.scaleway.ai/v1/", *app.Environment[endpointKey])
				assert.Equal(t, model.resolved, *app.Environment[modelKey])
				assert.Contains(t, app.Environment, "OPENAI_API_KEY")
				assert.Nil(t, app.Environment["OPENAI_API_KEY"])
				assert.Empty(t, app.Models)
				assert.NotContains(t, app.DependsOn, "llm")
				assert.NotContains(t, project.Networks, "model_provider_private")

				fixed, err := compose.MarshalYAML(project)
				require.NoError(t, err)
				_, err = compose.LoadFromContent(t.Context(), fixed, "")
				require.NoError(t, err, "the deploy payload must have no orphan model references")
				require.NoError(t, compose.FixupServices(t.Context(), provider, project, compose.UploadModeIgnore))
				again, err := compose.MarshalYAML(project)
				require.NoError(t, err)
				assert.Equal(t, string(fixed), string(again))
			})
		}
	}
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
