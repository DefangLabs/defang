package modes

import (
	"fmt"
	"slices"
	"strings"

	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type Mode defangv1.DeploymentMode

func (b Mode) String() string {
	if b == 0 {
		return ""
	}

	switch defangv1.DeploymentMode(b) {
	case defangv1.DeploymentMode_DEVELOPMENT:
		return "AFFORDABLE"
	case defangv1.DeploymentMode_STAGING:
		return "BALANCED"
	case defangv1.DeploymentMode_PRODUCTION:
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
	enumKeys := make([]int32, 0, len(defangv1.DeploymentMode_name))
	for k := range defangv1.DeploymentMode_name {
		enumKeys = append(enumKeys, k)
	}

	slices.Sort(enumKeys)
	var modes []string
	for _, n := range enumKeys {
		if n == 0 {
			continue
		}
		name := strings.ToUpper(Mode(defangv1.DeploymentMode(n)).String())
		modes = append(modes, name)
	}
	return modes
}
