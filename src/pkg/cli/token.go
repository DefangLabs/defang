package cli

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/defang-io/defang/src/pkg/cli/client"
	"github.com/defang-io/defang/src/pkg/github"
	"github.com/defang-io/defang/src/pkg/scope"
	"github.com/defang-io/defang/src/pkg/types"
	v1 "github.com/defang-io/defang/src/protos/io/defang/v1"
)

func Token(ctx context.Context, client client.Client, clientId string, tenant types.TenantID, dur time.Duration, scope scope.Scope) error {
	if DoDryRun {
		return errors.New("dry-run")
	}

	code, err := github.StartAuthCodeFlow(ctx, clientId)
	if err != nil {
		return err
	}

	at, err := exchangeCodeForToken(ctx, client, code, tenant, dur, scope)
	if err != nil {
		return err
	}

	Print(BrightCyan, "Scoped access token: ")
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

	Debug(" - Generating token for tenant", tenant, "with scopes", scopes)

	token, err := client.Token(ctx, &v1.TokenRequest{AuthCode: code, Tenant: string(tenant), Scope: scopes, ExpiresIn: uint32(dur.Seconds())})
	if err != nil {
		return "", err
	}
	return token.AccessToken, nil
}
