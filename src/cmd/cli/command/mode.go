package command

import (
	"fmt"
	"strings"

	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type Mode defangv1.DeploymentMode

func (b Mode) String() string {
	return strings.ToLower(defangv1.DeploymentMode_name[int32(b)])
}
func (b *Mode) Set(s string) error {
	mode, ok := defangv1.DeploymentMode_value[strings.ToUpper(s)]
	if !ok {
		return fmt.Errorf("invalid mode: %s, valid values are: %v", s, strings.Join(allModes(), ", "))
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
	return modes
}
