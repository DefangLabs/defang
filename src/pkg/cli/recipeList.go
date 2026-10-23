package cli

import (
	"context"
	"fmt"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
)

func RecipeList(ctx context.Context, fabric client.FabricClient) error {
	resp, err := fabric.ListRecipes(ctx)
	if err != nil {
		return fmt.Errorf("failed to list recipes: %w", err)
	}

	if len(resp.Recipes) == 0 {
		term.Warn("No recipes found in this workspace.")
		return nil
	}

	return term.Table(resp.Recipes, "Name", "Active")
}
