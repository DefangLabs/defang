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
	return strings.ToLower(b.Value().String())
}

func (b *Mode) Set(s string) error {
	mode, ok := defangv1.DeploymentMode_value[strings.ToUpper(s)]
	if !ok {
		return fmt.Errorf("deployment mode not one of %v", AllModes())
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

func AllModes() []Mode {
	modes := make([]Mode, 0, len(defangv1.DeploymentMode_name)-1)
	for i := range defangv1.DeploymentMode_name {
		if i == 0 {
			continue // skip the zero/unspecified value
		}
		modes = append(modes, Mode(i))
	}
	return modes
}
