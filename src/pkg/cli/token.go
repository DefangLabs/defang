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

func Token(ctx context.Context, client client.FabricClient, tenant types.TenantName, dur time.Duration, s scope.Scope) error {
	if DoDryRun {
		return ErrDryRun
	}

	code, err := auth.StartAuthCodeFlow(ctx, true)
	if err != nil {
		return err
	}

	at, err := auth.ExchangeCodeForToken(ctx, code, tenant, dur, s)
	if err != nil {
		return err
	}

	// Translate the OpenAuth token to our own Defang Fabric token
	var scopes []string
	if s != scope.Admin {
		scopes = []string{string(s)}
	}

	resp, err := client.Token(ctx, &defangv1.TokenRequest{
		Assertion: at,
		ExpiresIn: uint32(dur.Seconds()),
		Scope:     scopes,
		Tenant:    string(tenant),
	})
	if err != nil {
		return err
	}

	term.Printc(term.BrightCyan, "Scoped access token: ")
	term.Println(resp.AccessToken)
	return nil
}
