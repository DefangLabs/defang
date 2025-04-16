package cli

import (
	"context"
	"time"

	"github.com/DefangLabs/defang/src/pkg/auth"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/scope"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
)

func Token(ctx context.Context, client client.FabricClient, clientId string, tenant types.TenantName, dur time.Duration, scope scope.Scope) error {
	if DoDryRun {
		return ErrDryRun
	}

	code, err := auth.StartAuthCodeFlow(ctx, clientId)
	if err != nil {
		return err
	}

	at, err := auth.ExchangeCodeForToken(ctx, code, tenant, dur, scope)
	if err != nil {
		return err
	}

	term.Printc(term.BrightCyan, "Scoped access token: ")
	term.Println(at)
	return nil
}
