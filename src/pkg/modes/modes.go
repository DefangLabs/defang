package modes

import (
	"strings"

	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type Mode string

const (
	ModeUnspecified      Mode = Mode("")
	ModeAffordable       Mode = Mode("AFFORDABLE")
	ModeBalanced         Mode = Mode("BALANCED")
	ModeHighAvailability Mode = Mode("HIGH_AVAILABILITY")
)

func (m Mode) String() string {
	return string(m)
}

func (m *Mode) Set(s string) error {
	*m = Parse(s)
	return nil
}

func Parse(str string) Mode {
	upper := strings.ToUpper(str)
	// Handle legacy aliases
	switch upper {
	case "CHEAP", "DEVELOPMENT":
		return ModeAffordable
	case "STAGING":
		return ModeBalanced
	case "HA", "HIGH-AVAILABILITY", "PRODUCTION":
		return ModeHighAvailability
	}
	return Mode(upper)
}

func (Mode) Type() string {
	return "mode"
}

func (m Mode) Value() defangv1.DeploymentMode {
	switch m {
	case ModeAffordable:
		return defangv1.DeploymentMode_DEVELOPMENT
	case ModeBalanced:
		return defangv1.DeploymentMode_STAGING
	case ModeHighAvailability:
		return defangv1.DeploymentMode_PRODUCTION
	default:
		return defangv1.DeploymentMode_MODE_UNSPECIFIED
	}
}

// Deprecated: replaced by free-form recipe names, ListRecipes gRPC method
func AllDeploymentModes() []string {
	return []string{
		ModeAffordable.String(),
		ModeBalanced.String(),
		ModeHighAvailability.String(),
	}
}
