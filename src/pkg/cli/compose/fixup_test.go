package compose

import (
	"strings"
	"testing"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	composeTypes "github.com/compose-spec/compose-go/v2/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
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

		actual, err := yaml.Marshal(services)
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

func TestAccessGatewayModelAliases(t *testing.T) {
	tests := []struct {
		name         string
		provider     client.ProviderID
		model        string
		wantModel    string
		wantAlias    string
		wantLocation string
	}{
		{name: "AWS chat-default", provider: client.ProviderAWS, model: "chat-default", wantModel: "bedrock/us.amazon.nova-2-lite-v1:0", wantAlias: "chat-default"},
		{name: "AWS ai/chat-default", provider: client.ProviderAWS, model: "ai/chat-default", wantModel: "bedrock/us.amazon.nova-2-lite-v1:0", wantAlias: "ai/chat-default"},
		{name: "AWS embedding-default", provider: client.ProviderAWS, model: "embedding-default", wantModel: "bedrock/amazon.titan-embed-text-v2:0", wantAlias: "embedding-default"},
		{name: "AWS ai/embedding-default", provider: client.ProviderAWS, model: "ai/embedding-default", wantModel: "bedrock/amazon.titan-embed-text-v2:0", wantAlias: "ai/embedding-default"},
		{name: "AWS chat-large", provider: client.ProviderAWS, model: "chat-large", wantModel: "bedrock/us.anthropic.claude-sonnet-5", wantAlias: "chat-large"},
		{name: "AWS ai/chat-large", provider: client.ProviderAWS, model: "ai/chat-large", wantModel: "bedrock/us.anthropic.claude-sonnet-5", wantAlias: "ai/chat-large"},
		{name: "AWS non-alias ai model", provider: client.ProviderAWS, model: "ai/custom", wantModel: "ai/custom", wantAlias: "ai/custom"},
		{name: "GCP chat-default", provider: client.ProviderGCP, model: "chat-default", wantModel: "vertex_ai/gemini-2.5-flash", wantAlias: "chat-default", wantLocation: "us-central1"},
		{name: "GCP ai/chat-default", provider: client.ProviderGCP, model: "ai/chat-default", wantModel: "vertex_ai/gemini-2.5-flash", wantAlias: "ai/chat-default", wantLocation: "us-central1"},
		{name: "GCP embedding-default", provider: client.ProviderGCP, model: "embedding-default", wantModel: "vertex_ai/gemini-embedding-001", wantAlias: "embedding-default", wantLocation: "us-central1"},
		{name: "GCP ai/embedding-default", provider: client.ProviderGCP, model: "ai/embedding-default", wantModel: "vertex_ai/gemini-embedding-001", wantAlias: "ai/embedding-default", wantLocation: "us-central1"},
		{name: "GCP chat-large", provider: client.ProviderGCP, model: "chat-large", wantModel: "vertex_ai/gemini-3.1-pro-preview", wantAlias: "chat-large", wantLocation: "global"},
		{name: "GCP ai/chat-large", provider: client.ProviderGCP, model: "ai/chat-large", wantModel: "vertex_ai/gemini-3.1-pro-preview", wantAlias: "ai/chat-large", wantLocation: "global"},
		{name: "GCP non-alias ai model", provider: client.ProviderGCP, model: "ai/custom", wantModel: "ai/custom", wantAlias: "ai/custom", wantLocation: "us-central1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := &client.AccountInfo{
				Provider:  tt.provider,
				Region:    "us-central1",
				AccountID: "my-gcp-project",
			}
			proj := &composeTypes.Project{Networks: map[string]composeTypes.NetworkConfig{}, Services: composeTypes.Services{}}
			svccfg := newLLMService()
			makeAccessGatewayService(&svccfg, proj, tt.model, info)

			require.Equal(t, []string{"--drop_params", "--model", tt.wantModel, "--alias", tt.wantAlias}, []string(svccfg.Command))
			if tt.provider == client.ProviderAWS {
				assert.Equal(t, "us-central1", *svccfg.Environment["AWS_REGION"])
			} else {
				assert.Equal(t, "my-gcp-project", *svccfg.Environment["VERTEXAI_PROJECT"])
				assert.Equal(t, tt.wantLocation, *svccfg.Environment["VERTEXAI_LOCATION"])
			}
		})
	}
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
	t.Run("models binding injects OPENAI_API_KEY into consuming service", func(t *testing.T) {
		masterKey := "model-binding-key"
		proj := &composeTypes.Project{
			Networks: map[string]composeTypes.NetworkConfig{},
			Services: composeTypes.Services{
				"app": {
					Name:     "app",
					Image:    "myapp",
					Models:   map[string]*composeTypes.ServiceModelConfig{"llm": nil},
					Networks: map[string]*composeTypes.ServiceNetworkConfig{},
				},
			},
		}
		svccfg := newLLMService()
		svccfg.Environment["LITELLM_MASTER_KEY"] = &masterKey
		makeAccessGatewayService(&svccfg, proj, "ai/model", info)

		app := proj.Services["app"]
		require.Contains(t, app.Environment, "OPENAI_API_KEY")
		assert.Equal(t, masterKey, *app.Environment["OPENAI_API_KEY"])
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

func TestAccessGatewayMemoryDefault(t *testing.T) {
	tests := []struct {
		name       string
		memory     composeTypes.UnitBytes
		wantMemory composeTypes.UnitBytes
	}{
		{
			name:       "unset memory gets default",
			wantMemory: defaultLLMMemoryMiB * MiB,
		},
		{
			name:       "existing memory is preserved",
			memory:     1024 * MiB,
			wantMemory: 1024 * MiB,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proj := &composeTypes.Project{Networks: map[string]composeTypes.NetworkConfig{}, Services: composeTypes.Services{}}
			svccfg := newLLMService()
			if tt.memory != 0 {
				svccfg.Deploy = &composeTypes.DeployConfig{
					Resources: composeTypes.Resources{
						Reservations: &composeTypes.Resource{MemoryBytes: tt.memory},
					},
				}
			}

			makeAccessGatewayService(&svccfg, proj, "ai/model", &client.AccountInfo{})

			require.NotNil(t, svccfg.Deploy)
			require.NotNil(t, svccfg.Deploy.Resources.Reservations)
			assert.Equal(t, tt.wantMemory, svccfg.Deploy.Resources.Reservations.MemoryBytes)
		})
	}
}

func TestFixupLLM(t *testing.T) {
	tests := []struct {
		name          string
		image         string
		existingPorts []composeTypes.ServicePortConfig
		wantPort      bool
	}{
		{
			name:     "registry with port and tag adds litellm port",
			image:    "registry.example:5000/litellm:latest",
			wantPort: true,
		},
		{
			name:     "registry with port and no tag adds litellm port",
			image:    "registry.example:5000/litellm",
			wantPort: true,
		},
		{
			name:     "standard registry with path and tag adds litellm port",
			image:    "ghcr.io/berriai/litellm:main-latest",
			wantPort: true,
		},
		{
			name:     "image with digest adds litellm port",
			image:    "ghcr.io/berriai/litellm@sha256:abc123",
			wantPort: true,
		},
		{
			name:     "non-litellm image does not add port",
			image:    "registry.example:5000/other:tag",
			wantPort: false,
		},
		{
			name:  "litellm image with existing ports does not add port",
			image: "ghcr.io/berriai/litellm:main-latest",
			existingPorts: []composeTypes.ServicePortConfig{
				{Target: 8080, Mode: Mode_HOST, Protocol: Protocol_TCP},
			},
			wantPort: false,
		},
		{
			name:     "bare image name without slash does not match",
			image:    "litellm:latest",
			wantPort: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := composeTypes.ServiceConfig{
				Name:  "llm",
				Image: tt.image,
				Ports: tt.existingPorts,
			}
			fixupLLM(&svc)
			if tt.wantPort {
				require.Len(t, svc.Ports, 1)
				assert.Equal(t, liteLLMPort, svc.Ports[0].Target)
				assert.Equal(t, Mode_HOST, svc.Ports[0].Mode)
				assert.Equal(t, Protocol_TCP, svc.Ports[0].Protocol)
			} else {
				assert.Equal(t, tt.existingPorts, svc.Ports)
			}
		})
	}
}

func TestModelWithProvider(t *testing.T) {
	assert.Equal(t, "bedrock/my-model", modelWithProvider("my-model", "bedrock"))
	assert.Equal(t, "bedrock/my-model", modelWithProvider("bedrock/my-model", "bedrock"))
	assert.Equal(t, "vertex_ai/gemini-2.5-flash", modelWithProvider("gemini-2.5-flash", "vertex_ai"))
	assert.Equal(t, "vertex_ai/gemini-2.5-flash", modelWithProvider("vertex_ai/gemini-2.5-flash", "vertex_ai"))
}
