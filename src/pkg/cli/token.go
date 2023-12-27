package cli

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/bufbuild/connect-go"
	"github.com/defang-io/defang/src/pkg"
	fab "github.com/defang-io/defang/src/pkg"
	"github.com/defang-io/defang/src/pkg/github"
	"github.com/defang-io/defang/src/pkg/scope"
	pb "github.com/defang-io/defang/src/protos/io/defang/v1"
	"github.com/defang-io/defang/src/protos/io/defang/v1/defangv1connect"
)

func Token(ctx context.Context, client defangv1connect.FabricControllerClient, clientId string, tenant pkg.TenantID, dur time.Duration, scope scope.Scope) error {
	if DoDryRun {
		return errors.New("dry-run")
	}

	code, err := github.StartAuthCodeFlow(ctx, clientId)
	if err != nil {
		return err
	}

	at, err := generateToken(ctx, client, code, tenant, dur, scope)
	if err != nil {
		return err
	}

	Print(BrightCyan, "Scoped access token: ")
	fmt.Println(at)
	return nil
}

func generateToken(ctx context.Context, client defangv1connect.FabricControllerClient, code string, tenant pkg.TenantID, dur time.Duration, ss ...scope.Scope) (string, error) {
	var scopes []string
	for _, s := range ss {
		if s == scope.Admin {
			scopes = nil
			break
		}
		scopes = append(scopes, s.String())
	}

	Debug(" - Generating token for tenant", tenant, "with scopes", scopes)

	token, err := client.Token(ctx, connect.NewRequest(&pb.TokenRequest{AuthCode: code, Tenant: string(tenant), Scope: scopes, ExpiresIn: uint32(dur.Seconds())}))
	if err != nil {
		return "", err
	}
	return token.Msg.AccessToken, nil
}

func TenantFromAccessToken(at string) (fab.TenantID, error) {
	parts := strings.Split(at, ".")
	if len(parts) != 3 {
		return "", errors.New("not a JWT")
	}
	var claims struct {
		Sub string `json:"sub"`
	}
	bytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", err
	}
	err = json.Unmarshal(bytes, &claims)
	return fab.TenantID(claims.Sub), err
}
