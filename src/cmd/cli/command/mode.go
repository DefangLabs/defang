package command

import (
	"fmt"
	"strings"

	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type Mode defangv1.DeploymentMode

func (b Mode) String() string {
	if b == 0 {
		return ""
	}
	return strings.ToLower(defangv1.DeploymentMode_name[int32(b)])
}

func NewMode(s string) (Mode, error) {
	upper := strings.ToUpper(s)
	mode, ok := defangv1.DeploymentMode_value[upper]
	if !ok {
		switch upper {
		case "AFFORDABLE":
			mode = int32(defangv1.DeploymentMode_DEVELOPMENT)
		case "BALANCED":
			mode = int32(defangv1.DeploymentMode_STAGING)
		case "HIGH_AVAILABILITY":
		case "HA":
			mode = int32(defangv1.DeploymentMode_PRODUCTION)
		default:
			return 0, fmt.Errorf("invalid mode: %s, not one of %v", s, allModes())
		}
	}
	return Mode(mode), nil
}

func (b *Mode) Set(s string) error {
	mode, err := NewMode(s)
	if err != nil {
		return fmt.Errorf("failed to set mode: %w", err)
	}
	*b = mode

	return nil
}

func (b Mode) Type() string {
	return "mode"
}

func (b Mode) Value() defangv1.DeploymentMode {
	return defangv1.DeploymentMode(b)
}

func allModes() []string {
	return []string{"affordable", "balanced", "high_availability"}
}
