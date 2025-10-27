package modes

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type Mode defangv1.DeploymentMode

const (
	ModeUnspecified      Mode = Mode(defangv1.DeploymentMode_MODE_UNSPECIFIED)
	ModeAffordable       Mode = Mode(defangv1.DeploymentMode_DEVELOPMENT)
	ModeBalanced         Mode = Mode(defangv1.DeploymentMode_STAGING)
	ModeHighAvailability Mode = Mode(defangv1.DeploymentMode_PRODUCTION)
)

func (b Mode) String() string {
	if b == 0 {
		return ""
	}

	switch b {
	case ModeAffordable:
		return "AFFORDABLE"
	case ModeBalanced:
		return "BALANCED"
	case ModeHighAvailability:
		return "HIGH_AVAILABILITY"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", b)
	}
}

func (b *Mode) Set(s string) error {
	mode, err := Parse(s)
	if err != nil {
		return err
	}
	*b = mode
	return nil
}

func Parse(s string) (Mode, error) {
	upper := strings.ToUpper(s)
	mode, ok := defangv1.DeploymentMode_value[upper]
	if !ok {
		switch upper {
		case "AFFORDABLE", "CHEAP":
			mode = int32(defangv1.DeploymentMode_DEVELOPMENT)
		case "BALANCED":
			mode = int32(defangv1.DeploymentMode_STAGING)
		case "HA", "HIGH_AVAILABILITY", "HIGH-AVAILABILITY":
			mode = int32(defangv1.DeploymentMode_PRODUCTION)
		default:
			return 0, fmt.Errorf("invalid mode: %s, not one of %v", s, AllDeploymentModes())
		}
	}
	return Mode(mode), nil
}

func (b Mode) Type() string {
	return "mode"
}

func (b Mode) Value() defangv1.DeploymentMode {
	return defangv1.DeploymentMode(b)
}

func AllDeploymentModes() []string {
	var modes []string
	for _, i := range slices.Sorted(maps.Keys(defangv1.DeploymentMode_name)) {
		if i == 0 {
			continue
		}
		modes = append(modes, Mode(i).String())
	}
	return modes
}
