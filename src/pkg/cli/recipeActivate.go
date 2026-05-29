package cli

import (
	"context"
	"fmt"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func RecipeActivate(ctx context.Context, fabric client.FabricClient, name string, active bool) error {
	resp, err := fabric.GetRecipe(ctx, &defangv1.GetRecipeRequest{Name: name})
	if err != nil {
		return fmt.Errorf("failed to get recipe: %w", err)
	}

	err = fabric.PutRecipe(ctx, &defangv1.PutRecipeRequest{
		Recipe: &defangv1.Recipe{
			Name:         resp.Recipe.Name,
			PulumiConfig: resp.Recipe.PulumiConfig,
			Active:       active,
		},
	})
	if err == nil {
		state := "active"
		if !active {
			state = "inactive"
		}
		term.Info(fmt.Sprintf("Recipe %q is now %s.", name, state))
	}
	return err
}
