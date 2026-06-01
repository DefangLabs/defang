package modes

import (
	"strings"

	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type Recipe string

const (
	RecipeUnspecified      Recipe = Recipe("")
	RecipeAffordable       Recipe = Recipe("AFFORDABLE")
	RecipeBalanced         Recipe = Recipe("BALANCED")
	RecipeHighAvailability Recipe = Recipe("HIGH_AVAILABILITY")
)

func (r Recipe) String() string {
	return string(r)
}

func (r *Recipe) Set(s string) error {
	*r = ParseRecipe(s)
	return nil
}

func ParseRecipe(str string) Recipe {
	upper := strings.ToUpper(str)
	// Handle legacy aliases
	switch upper {
	case "CHEAP", "DEVELOPMENT":
		return RecipeAffordable
	case "STAGING":
		return RecipeBalanced
	case "HA", "HIGH-AVAILABILITY", "PRODUCTION":
		return RecipeHighAvailability
	}
	return Recipe(upper)
}

func (Recipe) Type() string {
	return "recipe"
}

func (r Recipe) Mode() Mode {
	switch r {
	case RecipeAffordable:
		return ModeAffordable
	case RecipeBalanced:
		return ModeBalanced
	case RecipeHighAvailability:
		return ModeHighAvailability
	default:
		return ModeUnspecified
	}
}

// FromMode converts a protobuf DeploymentMode to a Recipe; it is the inverse of Mode.
func FromMode(mode defangv1.DeploymentMode) Recipe {
	switch mode {
	case defangv1.DeploymentMode_DEVELOPMENT:
		return RecipeAffordable
	case defangv1.DeploymentMode_STAGING:
		return RecipeBalanced
	case defangv1.DeploymentMode_PRODUCTION:
		return RecipeHighAvailability
	default:
		return RecipeUnspecified
	}
}
