package cli

import (
	"context"
	"os"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/bufbuild/connect-go"
)

func Logout(ctx context.Context, fabricClient client.FabricClient, cluster string) error {
	term.Debug("Logging out")
	err := fabricClient.RevokeToken(ctx)
	// Ignore unauthenticated errors, since we're logging out anyway
	if err != nil && connect.CodeOf(err) != connect.CodeUnauthenticated {
		return err
	}

	// Remove the cached token file
	tokenFile := client.GetTokenFile(cluster)
	if err := os.Remove(tokenFile); err != nil && !os.IsNotExist(err) {
		term.Warn("Failed to remove token file:", err)
		// Don't return the error - we still consider logout successful
	} else if err == nil {
		term.Debug("Removed token file:", tokenFile)
	}

	// Also remove the JWT web identity token file if it exists
	jwtFile, err := client.GetWebIdentityTokenFile(cluster)
	if err == nil {
		if err := os.Remove(jwtFile); err != nil && !os.IsNotExist(err) {
			term.Warn("Failed to remove JWT token file:", err)
		} else if err == nil {
			term.Debug("Removed JWT token file:", jwtFile)
		}
	}

	return nil
}
