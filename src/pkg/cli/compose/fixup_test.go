package compose

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
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

func TestFixupPortUDPMode(t *testing.T) {
	explicitIngress := fixupPort(composeTypes.ServicePortConfig{
		Target: 53, Mode: Mode_INGRESS, Protocol: Protocol_UDP,
	})
	assert.Equal(t, Mode_INGRESS, explicitIngress.Mode)
	assert.Empty(t, explicitIngress.AppProtocol)

	unspecified := fixupPort(composeTypes.ServicePortConfig{Target: 53, Protocol: Protocol_UDP})
	assert.Equal(t, Mode_INGRESS, unspecified.Mode)

	explicitHost := fixupPort(composeTypes.ServicePortConfig{
		Target: 53, Mode: Mode_HOST, Protocol: Protocol_UDP,
	})
	assert.Equal(t, Mode_HOST, explicitHost.Mode)
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

		require.Equal(t, []string{"--drop_params", "--model", "vertex_ai/gemini-3.5-flash", "--alias", "chat-default"}, []string(svccfg.Command))
		assert.Equal(t, "my-gcp-project", *svccfg.Environment["VERTEXAI_PROJECT"])
		assert.Equal(t, "global", *svccfg.Environment["VERTEXAI_LOCATION"])
	})

	t.Run("embedding-default model", func(t *testing.T) {
		proj := &composeTypes.Project{Networks: map[string]composeTypes.NetworkConfig{}, Services: composeTypes.Services{}}
		svccfg := newLLMService()
		makeAccessGatewayService(&svccfg, proj, "embedding-default", info)

		require.Equal(t, []string{"--drop_params", "--model", "vertex_ai/gemini-embedding-001", "--alias", "embedding-default"}, []string(svccfg.Command))
	})
}

func TestMakeAccessGatewayServiceAzure(t *testing.T) {
	// Azure's managed-model provider names the deployment after the alias and
	// selects the concrete model itself, so the CLI passes the bare alias through
	// unchanged (no bedrock/vertex_ai-style resolution or prefix) and sets no
	// provider-specific environment; the provider injects OPENAI_API_KEY.
	info := &client.AccountInfo{
		Provider:  client.ProviderAzure,
		Region:    "eastus",
		AccountID: "my-azure-sub",
	}

	for _, alias := range []string{"chat-default", "chat-large", "embedding-default", "some-custom-model"} {
		t.Run(alias, func(t *testing.T) {
			proj := &composeTypes.Project{Networks: map[string]composeTypes.NetworkConfig{}, Services: composeTypes.Services{}}
			svccfg := newLLMService()
			makeAccessGatewayService(&svccfg, proj, alias, info)

			require.Equal(t, []string{"--drop_params", "--model", alias, "--alias", alias}, []string(svccfg.Command))
			assert.NotContains(t, svccfg.Environment, "AWS_REGION")
			assert.NotContains(t, svccfg.Environment, "VERTEXAI_PROJECT")
			assert.NotContains(t, svccfg.Environment, "VERTEXAI_LOCATION")
		})
	}
}

func TestAccessGatewayChatLarge(t *testing.T) {
	tests := []struct {
		name         string
		provider     client.ProviderID
		wantModel    string
		wantLocation string
	}{
		{name: "AWS", provider: client.ProviderAWS, wantModel: "bedrock/zai.glm-5"},
		{name: "GCP", provider: client.ProviderGCP, wantModel: "vertex_ai/gemini-3.1-pro-preview", wantLocation: "global"},
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
			makeAccessGatewayService(&svccfg, proj, "chat-large", info)

			require.Equal(t, []string{"--drop_params", "--model", tt.wantModel, "--alias", "chat-large"}, []string(svccfg.Command))
			if tt.provider == client.ProviderAWS {
				assert.Equal(t, "us-central1", *svccfg.Environment["AWS_REGION"])
			} else if tt.provider == client.ProviderGCP {
				assert.Equal(t, "my-gcp-project", *svccfg.Environment["VERTEXAI_PROJECT"])
				assert.Equal(t, tt.wantLocation, *svccfg.Environment["VERTEXAI_LOCATION"])
			}
		})
	}
}

func TestMakeAccessGatewayServiceLiteLLMMasterKey(t *testing.T) {
	info := &client.AccountInfo{}

	t.Run("no OPENAI_API_KEY reads the key from config", func(t *testing.T) {
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
		makeAccessGatewayService(&svccfg, proj, "ai/model", info)

		require.Contains(t, svccfg.Environment, "LITELLM_MASTER_KEY")
		assert.Nil(t, svccfg.Environment["LITELLM_MASTER_KEY"])
		assert.Equal(t, "${LITELLM_MASTER_KEY}", *proj.Services["app"].Environment["OPENAI_API_KEY"])
	})

	t.Run("OPENAI_API_KEY from config on dependent service is interpolated", func(t *testing.T) {
		proj := &composeTypes.Project{
			Networks: map[string]composeTypes.NetworkConfig{},
			Services: composeTypes.Services{
				"app": {
					Name:  "app",
					Image: "myapp",
					DependsOn: map[string]composeTypes.ServiceDependency{
						"llm": {Condition: composeTypes.ServiceConditionStarted, Required: true},
					},
					Environment: composeTypes.MappingWithEquals{"OPENAI_API_KEY": nil},
					Networks:    map[string]*composeTypes.ServiceNetworkConfig{},
				},
			},
		}
		svccfg := newLLMService()
		makeAccessGatewayService(&svccfg, proj, "ai/model", info)

		assert.Equal(t, "${OPENAI_API_KEY}", *svccfg.Environment["LITELLM_MASTER_KEY"])
		assert.Nil(t, proj.Services["app"].Environment["OPENAI_API_KEY"])
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

	t.Run("LITELLM_MASTER_KEY from config on service is passed to consumers", func(t *testing.T) {
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
		svccfg.Environment["LITELLM_MASTER_KEY"] = nil
		makeAccessGatewayService(&svccfg, proj, "ai/model", info)

		assert.Equal(t, "${LITELLM_MASTER_KEY}", *proj.Services["app"].Environment["OPENAI_API_KEY"])
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

func TestAccessGatewayResourceDefaults(t *testing.T) {
	tests := []struct {
		name       string
		cpus       composeTypes.NanoCPUs
		memory     composeTypes.UnitBytes
		wantCPUs   composeTypes.NanoCPUs
		wantMemory composeTypes.UnitBytes
	}{
		{
			name:       "unset resources get defaults",
			wantCPUs:   defaultLLMCPUs,
			wantMemory: defaultLLMMemoryMiB * MiB,
		},
		{
			name:       "existing resources are preserved",
			cpus:       1,
			memory:     1024 * MiB,
			wantCPUs:   1,
			wantMemory: 1024 * MiB,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proj := &composeTypes.Project{Networks: map[string]composeTypes.NetworkConfig{}, Services: composeTypes.Services{}}
			svccfg := newLLMService()
			if tt.cpus != 0 || tt.memory != 0 {
				svccfg.Deploy = &composeTypes.DeployConfig{
					Resources: composeTypes.Resources{
						Reservations: &composeTypes.Resource{NanoCPUs: tt.cpus, MemoryBytes: tt.memory},
					},
				}
			}

			makeAccessGatewayService(&svccfg, proj, "ai/model", &client.AccountInfo{})

			require.NotNil(t, svccfg.Deploy)
			require.NotNil(t, svccfg.Deploy.Resources.Reservations)
			assert.Equal(t, tt.wantCPUs, svccfg.Deploy.Resources.Reservations.NanoCPUs)
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
	assert.Equal(t, "vertex_ai/gemini-3.5-flash", modelWithProvider("gemini-3.5-flash", "vertex_ai"))
	assert.Equal(t, "vertex_ai/gemini-3.5-flash", modelWithProvider("vertex_ai/gemini-3.5-flash", "vertex_ai"))
}

func TestFixupImageAndBuildPullPolicy(t *testing.T) {
	tests := []struct {
		name       string
		pullPolicy string
		upload     UploadMode
		wantBuild  bool
	}{
		{
			name:       "image with build and no pull_policy uses published image",
			pullPolicy: "",
			upload:     UploadModeIgnore,
			wantBuild:  false,
		},
		{
			name:       "image with build and pull_policy build keeps build",
			pullPolicy: composeTypes.PullPolicyBuild,
			upload:     UploadModeIgnore,
			wantBuild:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proj := &composeTypes.Project{
				Name: "test",
				Services: composeTypes.Services{
					"app": composeTypes.ServiceConfig{
						Name:       "app",
						Image:      "example/app:latest",
						Build:      &composeTypes.BuildConfig{Context: "."},
						PullPolicy: tt.pullPolicy,
					},
				},
			}

			err := FixupServices(t.Context(), &client.MockProvider{}, proj, tt.upload)
			require.NoError(t, err)

			svc := proj.Services["app"]
			if tt.wantBuild {
				assert.NotNil(t, svc.Build, "expected build config to be preserved")
			} else {
				assert.Nil(t, svc.Build, "expected build config to be dropped in favor of the published image")
			}
		})
	}
}

type gatewayKeyProvider struct {
	client.MockProvider
	configs []string
	listErr error
	puts    []*defangv1.PutConfigRequest
}

func (p *gatewayKeyProvider) ListConfig(context.Context, *defangv1.ListConfigsRequest) (*defangv1.Secrets, error) {
	return &defangv1.Secrets{Names: p.configs}, p.listErr
}

func (p *gatewayKeyProvider) PutConfig(_ context.Context, req *defangv1.PutConfigRequest) error {
	p.puts = append(p.puts, req)
	return nil
}

func TestFixupServicesGatewayKey(t *testing.T) {
	appKey := "sk-app" //nolint:gosec
	tests := []struct {
		name       string
		configs    []string
		listErr    error
		upload     UploadMode
		appEnv     composeTypes.MappingWithEquals
		deprecated bool // provider: type: model instead of models:
		wantErr    bool
		wantCreate bool
	}{
		{name: "creates a missing key", upload: UploadModeDigest, wantCreate: true},
		{name: "creates a missing key for the deprecated provider syntax", deprecated: true, upload: UploadModeDigest, wantCreate: true},
		{name: "creates a missing key for a preview", upload: UploadModePreview, wantCreate: true},
		{name: "keeps an existing key", configs: []string{"LITELLM_MASTER_KEY"}, upload: UploadModeDigest},
		{name: "dry run creates nothing", upload: UploadModeIgnore},
		{name: "estimate creates nothing", upload: UploadModeEstimate},
		{name: "config list failure is an error", listErr: errors.New("denied"), upload: UploadModeDigest, wantErr: true},
		{name: "app key needs no config", upload: UploadModeDigest, appEnv: composeTypes.MappingWithEquals{"OPENAI_API_KEY": &appKey}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &gatewayKeyProvider{configs: tt.configs, listErr: tt.listErr}
			project := &composeTypes.Project{
				Name:     "proj",
				Networks: composeTypes.Networks{},
				Models:   composeTypes.Models{"llm": {Model: "ai/model"}},
				Services: composeTypes.Services{
					"app": {
						Name:        "app",
						Image:       "myapp",
						DependsOn:   composeTypes.DependsOnConfig{"llm": {Condition: composeTypes.ServiceConditionStarted, Required: true}},
						Environment: tt.appEnv,
						Networks:    map[string]*composeTypes.ServiceNetworkConfig{},
					},
				},
			}
			if tt.deprecated {
				project.Models = nil
				project.Services["llm"] = composeTypes.ServiceConfig{
					Name:     "llm",
					Provider: &composeTypes.ServiceProviderConfig{Type: "model", Options: composeTypes.MultiOptions{"model": {"ai/model"}}},
				}
			}

			err := FixupServices(t.Context(), provider, project, tt.upload)
			if tt.wantErr {
				require.Error(t, err)
				assert.Empty(t, provider.puts, "an existing key must never be overwritten")
				return
			}
			require.NoError(t, err)
			if !tt.wantCreate {
				assert.Empty(t, provider.puts)
				return
			}
			require.Len(t, provider.puts, 1)
			put := provider.puts[0]
			assert.Equal(t, "proj", put.Project)
			assert.Equal(t, "LITELLM_MASTER_KEY", put.Name)
			assert.GreaterOrEqual(t, len(put.Value), 26)
			assert.Nil(t, project.Services["llm"].Environment["LITELLM_MASTER_KEY"])
			assert.Equal(t, "${LITELLM_MASTER_KEY}", *project.Services["app"].Environment["OPENAI_API_KEY"])
		})
	}
}
