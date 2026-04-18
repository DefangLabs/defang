package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"connectrpc.com/connect"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
)

func Logout(ctx context.Context, fabricClient client.FabricClient, fabricAddr string) error {
	slog.Debug("Logging out")
	err := fabricClient.RevokeToken(ctx)
	// Ignore unauthenticated errors, since we're logging out anyway
	if err != nil && connect.CodeOf(err) != connect.CodeUnauthenticated {
		return err
	}

	if err := client.TokenStore.Delete(client.TokenStorageName(fabricAddr)); err != nil {
		slog.Warn(fmt.Sprintln("Failed to remove stored token:", err))
		// Don't return the error - we still consider logout successful
	}

	// Also remove the JWT web identity token file if it exists
	jwtFile, err := client.GetWebIdentityTokenFile(fabricAddr)
	if err == nil {
		if err := os.Remove(jwtFile); err != nil && !os.IsNotExist(err) {
			slog.Warn(fmt.Sprintln("Failed to remove JWT token file:", err))
		} else if err == nil {
			slog.Debug(fmt.Sprintln("Removed JWT token file:", jwtFile))
		}
	}

	return nil
}
