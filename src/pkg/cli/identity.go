package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/dryrun"
	"github.com/DefangLabs/defang/src/pkg/identity"
	"github.com/DefangLabs/defang/src/pkg/term"
)

// IdentityDomain is where tenant issuers live; DEFANG_ID_DOMAIN overrides it
// for testing against a non-production deployment.
func IdentityDomain() string {
	return pkg.Getenv("DEFANG_ID_DOMAIN", identity.DefaultDomain)
}

// IdentityKeyDir returns where this machine's private key for a tenant lives.
// One key per machine and tenant: every enabled key in a tenant can sign for
// that tenant's issuer, so there is nothing to gain from more.
func IdentityKeyDir(tenant string) string {
	return filepath.Join(client.StateDir, "identity", IdentityDomain(), tenant)
}

func IdentityClaim(ctx context.Context, accessToken, tenant string) error {
	if dryrun.DoDryRun {
		return dryrun.ErrDryRun
	}
	if _, err := identity.IssuerURL(IdentityDomain(), tenant); err != nil {
		return err
	}
	claimed, err := identity.NewClient(IdentityDomain(), accessToken).ClaimTenant(ctx, tenant)
	if err != nil {
		return err
	}
	term.Info("Tenant:", claimed.Name)
	term.Info("Issuer:", claimed.Issuer)
	return nil
}

func IdentityRegister(ctx context.Context, accessToken, tenant, label string, ttl time.Duration) error {
	if dryrun.DoDryRun {
		return dryrun.ErrDryRun
	}
	issuer, err := identity.IssuerURL(IdentityDomain(), tenant)
	if err != nil {
		return err
	}
	if label == "" {
		label, _ = os.Hostname()
	}

	keyDir := IdentityKeyDir(tenant)
	key, err := identity.LoadOrGenerateKey(keyDir)
	if err != nil {
		return fmt.Errorf("failed to load or generate keypair: %w", err)
	}
	term.Debugf("Using keypair in %s", keyDir)

	proof, err := key.ProofJWT(time.Now(), issuer)
	if err != nil {
		return fmt.Errorf("failed to sign proof of possession: %w", err)
	}
	registered, err := identity.NewClient(IdentityDomain(), accessToken).Register(ctx, tenant, identity.RegisterRequest{
		JWK:        key.PublicJWK(),
		Proof:      proof,
		Label:      label,
		TTLSeconds: int(ttl.Seconds()),
	})
	if err != nil {
		return err
	}

	term.Printc(term.BrightCyan, "Enabled public key: ")
	term.Println(registered.Kid)
	term.Info("Issuer:", registered.Issuer)
	term.Info("Every key enabled for this tenant can sign tokens for this issuer; a cloud trust policy cannot tell them apart.")
	return nil
}

func IdentityList(ctx context.Context, accessToken, tenant string) error {
	if _, err := identity.IssuerURL(IdentityDomain(), tenant); err != nil {
		return err
	}
	keys, err := identity.NewClient(IdentityDomain(), accessToken).List(ctx, tenant)
	if err != nil {
		return err
	}
	if len(keys) == 0 {
		term.Info("No keys enabled")
		return nil
	}
	for _, key := range keys {
		state := "active"
		switch {
		case key.RevokedAt != nil:
			state = "revoked " + *key.RevokedAt
		case !key.Active && key.ExpiresAt != nil:
			state = "expired " + *key.ExpiresAt
		case key.ExpiresAt != nil:
			state = "active until " + *key.ExpiresAt
		}
		label := ""
		if key.Label != nil {
			label = *key.Label
		}
		term.Printf("%s %s %q %s\n", key.Kid, key.Alg, label, state)
	}
	return nil
}

func IdentityRevoke(ctx context.Context, accessToken, tenant, kid string) error {
	if dryrun.DoDryRun {
		return dryrun.ErrDryRun
	}
	if _, err := identity.IssuerURL(IdentityDomain(), tenant); err != nil {
		return err
	}
	if err := identity.NewClient(IdentityDomain(), accessToken).Revoke(ctx, tenant, kid); err != nil {
		return err
	}
	term.Info("Revoked key", kid)
	return nil
}
