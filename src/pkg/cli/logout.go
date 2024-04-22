package cli

import (
	"context"

	"github.com/bufbuild/connect-go"
	"github.com/defang-io/defang/src/pkg/cli/client"
	"github.com/defang-io/defang/src/pkg/term"
)

func Logout(ctx context.Context, client client.Client) error {
	term.Debug(" - Logging out")
	err := client.RevokeToken(ctx)
	// Ignore unauthenticated errors, since we're logging out anyway
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		return err
	}
	// TODO: remove the cached token file
	// tokenFile := getTokenFile(fabric)
	// if err := os.Remove(tokenFile); err != nil {
	// 	return err
	// }
	return nil
}
