package cli

import (
	"context"
	"slices"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/compose-spec/compose-go/v2/types"
)

type configOutput struct {
	Service     string `json:"service"`
	Environment string `json:"environment"`
	Value       string `json:"value,omitempty"`
	Source      Source `json:"source,omitempty"`
}

const configMaskedValue = "******"

type Source string

const (
	SourceComposeFile   Source = "Compose"
	SourceDefangConfig  Source = "Config"
	SourceInterpolation Source = "Config (interpolated)"
)

func (s Source) String() string {
	return string(s)
}

func maskTrailingConfigValue(value string) string {
	// Mask the value if it looks like a secret and only show the first 4 characters
	if len(value) <= 4 {
		return configMaskedValue
	}
	return value[:4] + strings.Repeat("*", 2)
}

// determineConfigSource determines the source of an environment variable
// and returns the appropriate source type and value to display.
func determineConfigSource(envKey string, envValue *string, defangConfigs map[string]struct{}) (Source, string) {
	// If the key itself is a defang config, mask it
	if _, isDefangConfig := defangConfigs[envKey]; isDefangConfig {
		return SourceDefangConfig, configMaskedValue
	}

	// If value is nil, it's from the compose file with empty value. This mean the user forgot to set with defang config.
	// ValidateProjectConfig will catch this later and tell the user to set it.
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

// printConfigResolutionSummary prints a summary of where each environment variable in the compose file is coming from (compose file, defang config, or interpolation).
// If redact is true, it will mask values that are from the compose file and look like secrets.
func printConfigResolutionSummary(project *types.Project, defangConfig []string, redact bool) error {
	configset := make(map[string]struct{})
	for _, name := range defangConfig {
		configset[name] = struct{}{}
	}

	projectEnvVars := []configOutput{}

	for serviceName, service := range project.Services {
		for envKey, envValue := range service.Environment {
			source, value := determineConfigSource(envKey, envValue, configset)
			if redact && source == SourceComposeFile {
				isSecret, _, err := compose.IsSecret(envKey, value)
				if err != nil {
					return err
				}

				if isSecret {
					value = maskTrailingConfigValue(value)
				}
			}
			projectEnvVars = append(projectEnvVars, configOutput{
				Service:     serviceName,
				Environment: envKey,
				Value:       value,
				Source:      source,
			})
		}
	}

	// Don't print table if there are no environment variables
	if len(projectEnvVars) == 0 {
		return nil
	}

	// Sort by Service, then by Environment within each service
	slices.SortFunc(projectEnvVars, func(a, b configOutput) int {
		if cmp := strings.Compare(a.Service, b.Service); cmp != 0 {
			return cmp
		}
		return strings.Compare(a.Environment, b.Environment)
	})

	projectEnvVars = slices.Compact(projectEnvVars)

	term.Info("Service environment variables resolution summary:")

	return term.Table(projectEnvVars, "Service", "Environment", "Source", "Value")
}

func printConfigSummaryAndValidate(ctx context.Context, provider client.Provider, project *compose.Project, redact bool) error {
	configs, err := provider.ListConfig(ctx, &defangv1.ListConfigsRequest{Project: project.Name})
	if err != nil {
		return err
	}

	err = printConfigResolutionSummary(project, configs.Names, redact)
	if err != nil {
		return err
	}

	err = compose.ValidateProjectConfig(project, configs.Names)
	if err != nil {
		return &ComposeError{err}
	}

	return nil
}

func PrintConfigSummaryAndValidate(ctx context.Context, provider client.Provider, project *compose.Project) error {
	return printConfigSummaryAndValidate(ctx, provider, project, false)
}

func PrintRedactedConfigSummaryAndValidate(ctx context.Context, provider client.Provider, project *compose.Project) error {
	return printConfigSummaryAndValidate(ctx, provider, project, true)
}
