package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/github"
	"github.com/DefangLabs/defang/src/pkg/scope"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func Token(ctx context.Context, client client.Client, clientId string, tenant types.TenantID, dur time.Duration, scope scope.Scope) error {
	if DoDryRun {
		return ErrDryRun
	}

	code, err := github.StartAuthCodeFlow(ctx, clientId)
	if err != nil {
		return err
	}

	at, err := exchangeCodeForToken(ctx, client, code, tenant, dur, scope)
	if err != nil {
		return err
	}

	term.Printc(term.BrightCyan, "Scoped access token: ")
	fmt.Println(at)
	return nil
}

func exchangeCodeForToken(ctx context.Context, client client.Client, code string, tenant types.TenantID, dur time.Duration, ss ...scope.Scope) (string, error) {
	var scopes []string
	for _, s := range ss {
		if s == scope.Admin {
			scopes = nil
			break
		}
		scopes = append(scopes, s.String())
	}

	term.Debug("Generating token for tenant", tenant, "with scopes", scopes)

	token, err := client.Token(ctx, &defangv1.TokenRequest{AuthCode: code, Tenant: string(tenant), Scope: scopes, ExpiresIn: uint32(dur.Seconds())})
	if err != nil {
		return "", err
	}
	return token.AccessToken, nil
}
