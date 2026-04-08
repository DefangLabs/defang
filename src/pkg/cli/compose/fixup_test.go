package compose

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	composeTypes "github.com/compose-spec/compose-go/v2/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFixup(t *testing.T) {
	testAllComposeFiles(t, func(t *testing.T, name, path string) {
		loader := NewLoader(WithPath(path))
		proj, err := loader.LoadProject(t.Context())
		if strings.HasPrefix(name, "invalid-") {
			assert.Error(t, err, "Expected error for invalid compose file: %s", path)
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		err = FixupServices(t.Context(), &client.MockProvider{}, proj, UploadModeIgnore)
		if err != nil {
			t.Fatal(err)
		}

		services := map[string]composeTypes.ServiceConfig{}
		for _, svc := range proj.Services {
			services[svc.Name] = svc
		}

		// Convert the protobuf services to pretty JSON for comparison (YAML would include all the zero values)
		actual, err := json.MarshalIndent(services, "", "  ")
		if err != nil {
			t.Fatal(err)
		}

		if err := pkg.Compare(actual, path+".fixup"); err != nil {
			t.Error(err)
		}
	})
}

func newLLMService() composeTypes.ServiceConfig {
	return composeTypes.ServiceConfig{
		Name:        "llm",
		Environment: composeTypes.MappingWithEquals{},
		Networks:    map[string]*composeTypes.ServiceNetworkConfig{},
	}
}

func TestMakeAccessGatewayServiceAWS(t *testing.T) {
	info := &client.AccountInfo{
		Provider:  client.ProviderAWS,
		Region:    "us-east-1",
		AccountID: "123456789",
	}

	t.Run("chat-default model", func(t *testing.T) {
		proj := &composeTypes.Project{Networks: map[string]composeTypes.NetworkConfig{}, Services: composeTypes.Services{}}
		svccfg := newLLMService()
		makeAccessGatewayService(&svccfg, proj, "chat-default", info)

		require.Equal(t, []string{"--drop_params", "--model", "bedrock/us.amazon.nova-2-lite-v1:0", "--alias", "chat-default"}, []string(svccfg.Command))
		assert.Equal(t, "us-east-1", *svccfg.Environment["AWS_REGION"])
	})

	t.Run("embedding-default model", func(t *testing.T) {
		proj := &composeTypes.Project{Networks: map[string]composeTypes.NetworkConfig{}, Services: composeTypes.Services{}}
		svccfg := newLLMService()
		makeAccessGatewayService(&svccfg, proj, "embedding-default", info)

		require.Equal(t, []string{"--drop_params", "--model", "bedrock/amazon.titan-embed-text-v2:0", "--alias", "embedding-default"}, []string(svccfg.Command))
	})

	t.Run("custom model gets bedrock prefix", func(t *testing.T) {
		proj := &composeTypes.Project{Networks: map[string]composeTypes.NetworkConfig{}, Services: composeTypes.Services{}}
		svccfg := newLLMService()
		makeAccessGatewayService(&svccfg, proj, "anthropic.claude-3-5-sonnet", info)

		require.Equal(t, []string{"--drop_params", "--model", "bedrock/anthropic.claude-3-5-sonnet", "--alias", "anthropic.claude-3-5-sonnet"}, []string(svccfg.Command))
	})

	t.Run("model with existing provider prefix is not double-prefixed", func(t *testing.T) {
		proj := &composeTypes.Project{Networks: map[string]composeTypes.NetworkConfig{}, Services: composeTypes.Services{}}
		svccfg := newLLMService()
		makeAccessGatewayService(&svccfg, proj, "bedrock/anthropic.claude-3-5-sonnet", info)

		require.Equal(t, []string{"--drop_params", "--model", "bedrock/anthropic.claude-3-5-sonnet", "--alias", "bedrock/anthropic.claude-3-5-sonnet"}, []string(svccfg.Command))
	})
}

func TestMakeAccessGatewayServiceGCP(t *testing.T) {
	info := &client.AccountInfo{
		Provider:  client.ProviderGCP,
		Region:    "us-central1",
		AccountID: "my-gcp-project",
	}

	t.Run("chat-default model", func(t *testing.T) {
		proj := &composeTypes.Project{Networks: map[string]composeTypes.NetworkConfig{}, Services: composeTypes.Services{}}
		svccfg := newLLMService()
		makeAccessGatewayService(&svccfg, proj, "chat-default", info)

		require.Equal(t, []string{"--drop_params", "--model", "vertex_ai/gemini-2.5-flash", "--alias", "chat-default"}, []string(svccfg.Command))
		assert.Equal(t, "my-gcp-project", *svccfg.Environment["VERTEXAI_PROJECT"])
		assert.Equal(t, "us-central1", *svccfg.Environment["VERTEXAI_LOCATION"])
	})

	t.Run("embedding-default model", func(t *testing.T) {
		proj := &composeTypes.Project{Networks: map[string]composeTypes.NetworkConfig{}, Services: composeTypes.Services{}}
		svccfg := newLLMService()
		makeAccessGatewayService(&svccfg, proj, "embedding-default", info)

		require.Equal(t, []string{"--drop_params", "--model", "vertex_ai/gemini-embedding-001", "--alias", "embedding-default"}, []string(svccfg.Command))
	})
}

func TestMakeAccessGatewayServiceLiteLLMMasterKey(t *testing.T) {
	info := &client.AccountInfo{}

	t.Run("no OPENAI_API_KEY uses default key", func(t *testing.T) {
		proj := &composeTypes.Project{Networks: map[string]composeTypes.NetworkConfig{}, Services: composeTypes.Services{}}
		svccfg := newLLMService()
		makeAccessGatewayService(&svccfg, proj, "ai/model", info)

		assert.Equal(t, "networkisalreadyprivate", *svccfg.Environment["LITELLM_MASTER_KEY"])
	})

	t.Run("OPENAI_API_KEY on dependent service propagates to LITELLM_MASTER_KEY", func(t *testing.T) {
		apiKey := "sk-my-secret-key" //nolint:gosec
		proj := &composeTypes.Project{
			Networks: map[string]composeTypes.NetworkConfig{},
			Services: composeTypes.Services{
				"app": {
					Name:  "app",
					Image: "myapp",
					DependsOn: map[string]composeTypes.ServiceDependency{
						"llm": {Condition: composeTypes.ServiceConditionStarted, Required: true},
					},
					Environment: composeTypes.MappingWithEquals{"OPENAI_API_KEY": &apiKey},
					Networks:    map[string]*composeTypes.ServiceNetworkConfig{},
				},
			},
		}
		svccfg := newLLMService()
		makeAccessGatewayService(&svccfg, proj, "ai/model", info)

		assert.Equal(t, "sk-my-secret-key", *svccfg.Environment["LITELLM_MASTER_KEY"])
	})

	t.Run("existing LITELLM_MASTER_KEY on service is preserved", func(t *testing.T) {
		proj := &composeTypes.Project{Networks: map[string]composeTypes.NetworkConfig{}, Services: composeTypes.Services{}}
		existingKey := "existing-master-key"
		svccfg := newLLMService()
		svccfg.Environment["LITELLM_MASTER_KEY"] = &existingKey
		makeAccessGatewayService(&svccfg, proj, "ai/model", info)

		assert.Equal(t, "existing-master-key", *svccfg.Environment["LITELLM_MASTER_KEY"])
	})
}

func TestModelWithProvider(t *testing.T) {
	assert.Equal(t, "bedrock/my-model", modelWithProvider("my-model", "bedrock"))
	assert.Equal(t, "bedrock/my-model", modelWithProvider("bedrock/my-model", "bedrock"))
	assert.Equal(t, "vertex_ai/gemini-2.5-flash", modelWithProvider("gemini-2.5-flash", "vertex_ai"))
	assert.Equal(t, "vertex_ai/gemini-2.5-flash", modelWithProvider("vertex_ai/gemini-2.5-flash", "vertex_ai"))
}
