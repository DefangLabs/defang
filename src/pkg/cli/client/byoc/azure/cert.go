package azure

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/clouds/azure/aca"
	"github.com/DefangLabs/defang/src/pkg/dns"
)

// IssueCert implements the cli.CertIssuer interface for the BYOD `defang cert
// generate` flow: it resolves the provider's subscription, credentials, and
// project resource group, then hands off to aca.IssueCert (cloud-SDK layer).
// The CD task also calls aca.IssueCert directly after a deploy to provision
// delegate-domain certs without depending on the CLI staying up. See
// pkg/clouds/azure/aca/cert.go for the shared cert flow.
func (b *ByocAzure) IssueCert(ctx context.Context, projectName, serviceName, hostname string, resolverAt func(string) dns.Resolver) error {
	cred, err := b.driver.NewCreds()
	if err != nil {
		return err
	}
	rg := b.projectResourceGroupName(projectName)
	return aca.IssueCert(ctx, cred, b.driver.SubscriptionID, rg, serviceName, hostname, resolverAt)
}
