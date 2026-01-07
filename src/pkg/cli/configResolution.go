package cli

import (
	"slices"
	"sort"

	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/compose-spec/compose-go/v2/types"
)

type configOutput struct {
	Service     string `json:"service"`
	Environment string `json:"environment"`
	Value       string `json:"value,omitempty"`
	Source      Source `json:"source,omitempty"`
}

const configMaskedValue = "*****"

type Source string

const (
	SourceComposeFile   Source = "Compose"
	SourceDefangConfig  Source = "Config"
	SourceInterpolation Source = "Config (interpolated)"
)

func (s Source) String() string {
	return string(s)
}

// determineConfigSource determines the source of an environment variable
// and returns the appropriate source type and value to display
func determineConfigSource(envKey string, envValue *string, defangConfigs map[string]struct{}) (Source, string) {
	// If the key itself is a defang config, mask it
	if _, isDefangConfig := defangConfigs[envKey]; isDefangConfig {
		return SourceDefangConfig, configMaskedValue
	}

	// If value is nil, it's from the compose file with empty value
	if envValue == nil {
		return SourceDefangConfig, ""
	}

	// Check if the value contains references to defang configs
	interpolatedVariables := compose.DetectInterpolationVariables(*envValue)
	if len(interpolatedVariables) > 0 {
		return SourceInterpolation, *envValue
	}

	// Otherwise, it's from the compose file
	return SourceComposeFile, *envValue
}

func PrintConfigResolutionSummary(project *types.Project, defangConfig []string) error {
	configset := make(map[string]struct{})
	for _, name := range defangConfig {
		configset[name] = struct{}{}
	}

	projectEnvVars := []configOutput{}

	for serviceName, service := range project.Services {
		for envKey, envValue := range service.Environment {
			source, value := determineConfigSource(envKey, envValue, configset)
			projectEnvVars = append(projectEnvVars, configOutput{
				Service:     serviceName,
				Environment: envKey,
				Value:       value,
				Source:      source,
			})
		}
	}

	// Sort by Service, then by Name within each service
	sort.Slice(projectEnvVars, func(i, j int) bool {
		if projectEnvVars[i].Service != projectEnvVars[j].Service {
			return projectEnvVars[i].Service < projectEnvVars[j].Service
		}
		return projectEnvVars[i].Environment < projectEnvVars[j].Environment
	})

	projectEnvVars = slices.Compact(projectEnvVars)

	// Don't print table if there are no environment variables
	if len(projectEnvVars) == 0 {
		return nil
	}

	// term.Println("\033[1mENVIRONMENT VARIABLES RESOLUTION SUMMARY:\033[0m")

	return term.Table(projectEnvVars, "Service", "Environment", "Value", "Source")
}
