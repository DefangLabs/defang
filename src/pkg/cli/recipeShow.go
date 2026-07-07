package cli

import (
	"context"
	"fmt"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func RecipeShow(ctx context.Context, fabric client.FabricClient, recipeName string) error {
	resp, err := fabric.GetRecipe(ctx, &defangv1.GetRecipeRequest{Name: recipeName})
	if err != nil {
		return fmt.Errorf("failed to get recipe: %w", err)
	}

	_, err = term.Println(resp.Recipe.GetPulumiConfig())
	return err
}
