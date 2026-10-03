package cli

import (
	"context"
	"time"

	"github.com/DefangLabs/defang/src/pkg/auth"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/dryrun"
	"github.com/DefangLabs/defang/src/pkg/scope"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func Token(ctx context.Context, fabric client.FabricClient, tenant types.TenantNameOrID, dur time.Duration, s scope.Scope, assertion string) (string, error) {
	if dryrun.DoDryRun {
		return "", dryrun.ErrDryRun
	}

	if assertion == "" {
		code, err := auth.StartAuthCodeFlow(ctx, false, func(token string) {
			term.Debug("Getting access token for scope:", s)
		}, "token-cli")
		if err != nil {
			return "", err
		}

		assertion, err = auth.ExchangeCodeForToken(ctx, code, s)
		if err != nil {
			return "", err
		}
	}

	// Translate the OpenAuth token to our own Defang Fabric token
	var scopes []string
	if s != scope.Admin {
		scopes = []string{string(s)}
	}

	term.Debugf("Generating token for tenant %q with scopes %v", tenant, scopes)

	resp, err := fabric.Token(ctx, &defangv1.TokenRequest{
		Assertion: assertion,
		ExpiresIn: uint32(dur.Seconds()),
		Scope:     scopes,
		Tenant:    string(tenant),
	})
	if err != nil {
		return "", err
	}

	return resp.AccessToken, nil
}
