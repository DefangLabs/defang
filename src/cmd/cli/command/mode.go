package command

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
	return strings.ToLower(defangv1.DeploymentMode_name[int32(b)])
}

func (b *Mode) Set(s string) error {
	upper := strings.ToUpper(s)
	mode, ok := defangv1.DeploymentMode_value[upper]
	if !ok {
		switch upper {
		case "CHEAP":
			mode = int32(defangv1.DeploymentMode_DEVELOPMENT)
		case "BALANCED":
			mode = int32(defangv1.DeploymentMode_STAGING)
		case "RELIABLE":
		case "RESILIENT":
			mode = int32(defangv1.DeploymentMode_PRODUCTION)
		default:
			return fmt.Errorf("invalid mode: %s, not one of %v", s, allModes())
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

func allModes() []string {
	modes := make([]string, 0, len(defangv1.DeploymentMode_name)-1)
	for i, mode := range defangv1.DeploymentMode_name {
		if i == 0 {
			continue
		}
		modes = append(modes, strings.ToLower(mode))
	}
	slices.Sort(modes) // TODO: sort by enum value instead of string
	return modes
}
