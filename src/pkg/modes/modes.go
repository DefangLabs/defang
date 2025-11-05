package modes

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

func (b *Mode) Set(s string) error {
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
			return fmt.Errorf("invalid mode: %s, not one of %v", s, AllDeploymentModes())
		}
	}
	*b = Mode(mode)
	return nil
}

func (b Mode) Type() string {
	return "mode"
}

func (b Mode) Value() defangv1.DeploymentMode {
	return defangv1.DeploymentMode(b)
}

func AllDeploymentModes() []string {
	return []string{"AFFORDABLE", "BALANCED", "HIGH_AVAILABILITY"}
}
