package command

import (
	"fmt"
	"strings"

	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type Behavior defangv1.Behavior

func (b Behavior) String() string {
	return defangv1.Behavior_name[int32(b)]
}
func (b *Behavior) Set(s string) error {
	behavior, ok := defangv1.Behavior_value[strings.ToUpper(s)]
	if !ok {
		return fmt.Errorf("invalid behavior: %s, valid values are: %v", s, strings.Join(allBehaviors(), ", "))
	}
	*b = Behavior(behavior)
	return nil
}
func (b Behavior) Type() string {
	return "behavior"
}

func (b Behavior) Value() defangv1.Behavior {
	return defangv1.Behavior(b)
}

func allBehaviors() []string {
	behaviors := make([]string, 0, len(defangv1.Behavior_name))
	for _, behavior := range defangv1.Behavior_name {
		behaviors = append(behaviors, strings.ToLower(behavior))
	}
	return behaviors
}
