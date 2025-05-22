package cli

import (
	"context"
	"time"

	"github.com/DefangLabs/defang/src/pkg/auth"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/scope"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func Token(ctx context.Context, client client.FabricClient, tenant types.TenantName, dur time.Duration, scope scope.Scope) error {
	if DoDryRun {
		return ErrDryRun
	}

	code, err := auth.StartAuthCodeFlow(ctx, auth.PromptYes, 0)
	if err != nil {
		return err
	}

	at, err := auth.ExchangeCodeForToken(ctx, code, tenant, dur, scope)
	if err != nil {
		return err
	}

	// Translate the OpenAuth token to our own Defang Fabric token
	resp, err := client.Token(ctx, &defangv1.TokenRequest{
		Tenant:    string(tenant),
		Scope:     []string{string(scope)},
		Assertion: at,
		ExpiresIn: uint32(dur.Seconds()),
	})
	if err != nil {
		return err
	}

	term.Printc(term.BrightCyan, "Scoped access token: ")
	term.Println(resp.AccessToken)
	return nil
}
