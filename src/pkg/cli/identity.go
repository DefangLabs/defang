package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/DefangLabs/defang/src/pkg/auth"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/dryrun"
	"github.com/DefangLabs/defang/src/pkg/identity"
	"github.com/DefangLabs/defang/src/pkg/term"
)

// identityRegistryClient builds a registry client for the fabric client's
// tenant, reusing the OpenAuth access token saved by `defang login`.
func identityRegistryClient(fabricClient client.FabricClient, accessToken string) (*identity.Client, error) {
	tenantURL, err := identity.TenantURL(auth.OpenAuthClient.Issuer(), string(fabricClient.GetTenantName()))
	if err != nil {
		return nil, err
	}
	return identity.NewClient(tenantURL, accessToken), nil
}

// IdentityKeyDir returns where the private key for a (tenant, project, stack)
// lives: one key per pair, because the registry rejects key reuse across
// stacks. The private key never leaves this directory.
func IdentityKeyDir(fabricClient client.FabricClient, projectName, stackName string) string {
	return filepath.Join(client.StateDir, "identity", string(fabricClient.GetTenantName()), projectName, stackName)
}

func IdentityRegister(ctx context.Context, fabricClient client.FabricClient, accessToken, projectName, stackName string, ttl time.Duration) error {
	if dryrun.DoDryRun {
		return dryrun.ErrDryRun
	}

	registry, err := identityRegistryClient(fabricClient, accessToken)
	if err != nil {
		return err
	}

	keyDir := IdentityKeyDir(fabricClient, projectName, stackName)
	key, err := identity.LoadOrGenerateKey(keyDir)
	if err != nil {
		return fmt.Errorf("failed to load or generate keypair: %w", err)
	}
	term.Debugf("Using keypair in %s", keyDir)

	popJwt, err := key.PopJWT(time.Now())
	if err != nil {
		return fmt.Errorf("failed to sign proof-of-possession: %w", err)
	}

	registered, err := registry.Register(ctx, identity.RegisterRequest{
		ProjectID:  projectName,
		StackID:    stackName,
		JWK:        key.PublicJWK(),
		PopJWT:     popJwt,
		TTLSeconds: int(ttl.Seconds()),
	})
	if err != nil {
		return err
	}

	term.Printc(term.BrightCyan, "Registered public key: ")
	term.Println(registered.Kid)
	term.Info("Subject:", registered.Subject)
	if registered.Issuer != "" {
		term.Info("Issuer:", registered.Issuer)
	}
	if registered.Expires > 0 {
		term.Info("Expires:", time.Unix(registered.Expires, 0).UTC().Format(time.RFC3339))
	}
	return nil
}

func IdentityList(ctx context.Context, fabricClient client.FabricClient, accessToken string) error {
	registry, err := identityRegistryClient(fabricClient, accessToken)
	if err != nil {
		return err
	}
	keys, err := registry.List(ctx)
	if err != nil {
		return err
	}
	if len(keys) == 0 {
		term.Info("No keys registered")
		return nil
	}
	for _, key := range keys {
		expires := ""
		if key.Expires > 0 {
			expires = " expires " + time.Unix(key.Expires, 0).UTC().Format(time.RFC3339)
		}
		term.Printf("%s project %q stack %q%s", key.Kid, key.ProjectID, key.StackID, expires)
	}
	return nil
}

func IdentityRevoke(ctx context.Context, fabricClient client.FabricClient, accessToken, kid string) error {
	if dryrun.DoDryRun {
		return dryrun.ErrDryRun
	}

	registry, err := identityRegistryClient(fabricClient, accessToken)
	if err != nil {
		return err
	}
	if err := registry.Revoke(ctx, kid); err != nil {
		return err
	}
	term.Info("Revoked key", kid)
	return nil
}
