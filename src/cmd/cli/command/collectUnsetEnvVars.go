package command

import (
	"errors"

	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	composeTypes "github.com/compose-spec/compose-go/v2/types"
)

func collectUnsetEnvVars(project *composeTypes.Project) []string {
	if project == nil {
		return nil // in case loading failed
	}
	err := compose.ValidateProjectConfig(project, []string{})
	var missingConfig compose.ErrMissingConfig
	if errors.As(err, &missingConfig) {
		return missingConfig
	}
	return nil
}
