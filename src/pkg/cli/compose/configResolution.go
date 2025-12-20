package compose

import (
	"slices"
	"sort"

	"github.com/DefangLabs/defang/src/pkg/term"
)

type configOutput struct {
	Service string `json:"service"`
	Name    string `json:"name"`
	Value   string `json:"value,omitempty"`
	Source  Source `json:"source,omitempty"`
}

type Source int

const (
	SourceUnknown Source = iota
	SourceComposeFile
	SourceDefangConfig
	SourceDefangAndComposeFile
)

var sourceNames = map[Source]string{
	SourceUnknown:              "unknown",
	SourceComposeFile:          "compose_file",
	SourceDefangConfig:         "defang_config",
	SourceDefangAndComposeFile: "compose_file and defang_config",
}

func (s Source) String() string {
	if name, ok := sourceNames[s]; ok {
		return name
	}
	return sourceNames[SourceUnknown]
}

// determineConfigSource determines the source of an environment variable
// and returns the appropriate source type and value to display
func determineConfigSource(envKey string, envValue *string, defangConfigs map[string]string) (Source, string) {
	// If the key itself is a defang config, mask it
	if _, isDefangConfig := defangConfigs[envKey]; isDefangConfig {
		return SourceDefangConfig, configMaskedValue
	}

	// If value is nil, it's from the compose file with empty value
	if envValue == nil {
		return SourceComposeFile, ""
	}

	// Check if the value contains references to defang configs
	interpolatedVariables := DetectInterpolationVariables(*envValue)
	if len(interpolatedVariables) > 0 {
		for _, varName := range interpolatedVariables {
			if _, isDefangConfig := defangConfigs[varName]; isDefangConfig {
				return SourceDefangAndComposeFile, *envValue
			}
		}
	}

	// Otherwise, it's from the compose file
	return SourceComposeFile, *envValue
}

const configMaskedValue = "*****"

func PrintConfigResolutionSummary(project Project, defangConfig []string) error {
	configset := make(map[string]string)
	for _, name := range defangConfig {
		configset[name] = ""
	}

	projectEnvVars := []configOutput{}

	for serviceName, service := range project.Services {
		for envKey, envValue := range service.Environment {
			source, value := determineConfigSource(envKey, envValue, configset)
			projectEnvVars = append(projectEnvVars, configOutput{
				Service: serviceName,
				Name:    envKey,
				Value:   value,
				Source:  source,
			})
		}
	}

	// Sort by Service, then by Name within each service
	sort.Slice(projectEnvVars, func(i, j int) bool {
		if projectEnvVars[i].Service != projectEnvVars[j].Service {
			return projectEnvVars[i].Service < projectEnvVars[j].Service
		}
		return projectEnvVars[i].Name < projectEnvVars[j].Name
	})

	projectEnvVars = slices.Compact(projectEnvVars)

	term.Println("\033[1mENVIRONMENT VARIABLES RESOLUTION SUMMARY:\033[0m")

	return term.Table(projectEnvVars, "Service", "Name", "Value", "Source")
}
