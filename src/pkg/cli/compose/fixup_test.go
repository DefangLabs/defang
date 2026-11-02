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

func TestMakeAccessGatewayServiceLiteLLMMasterKey(t *testing.T) {
	t.Run("no OPENAI_API_KEY uses default key", func(t *testing.T) {
		proj := &composeTypes.Project{Networks: map[string]composeTypes.NetworkConfig{}, Services: composeTypes.Services{}}
		svccfg := newLLMService()
		svccfg = makeLiteralGateway(svccfg, proj, "ai/model")

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
		svccfg = makeLiteralGateway(svccfg, proj, "ai/model")

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
		makeLiteralGateway(svccfg, proj, "ai/model")

		app := proj.Services["app"]
		require.Contains(t, app.Environment, "OPENAI_API_KEY")
		assert.Equal(t, masterKey, *app.Environment["OPENAI_API_KEY"])
	})

	t.Run("existing LITELLM_MASTER_KEY on service is preserved", func(t *testing.T) {
		proj := &composeTypes.Project{Networks: map[string]composeTypes.NetworkConfig{}, Services: composeTypes.Services{}}
		existingKey := "existing-master-key"
		svccfg := newLLMService()
		svccfg.Environment["LITELLM_MASTER_KEY"] = &existingKey
		svccfg = makeLiteralGateway(svccfg, proj, "ai/model")

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

			svccfg = makeLiteralGateway(svccfg, proj, "ai/model")

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
