package azure

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	armappcontainers "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appcontainers/armappcontainers/v3"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cert"
	"github.com/DefangLabs/defang/src/pkg/dns"
	"github.com/DefangLabs/defang/src/pkg/term"
)

const (
	// serviceTagKey matches the Pulumi provider's ServiceTags map key
	// (provider/defangazure/azure/azure.go: ServiceTags). Used to look up
	// the ContainerApp belonging to a Compose service without depending on
	// a deterministic name format.
	serviceTagKey = "defang-service"

	dnsWaitTimeout = 30 * time.Minute
	dnsPollEvery   = 15 * time.Second
	tlsWaitTimeout = 10 * time.Minute
	tlsPollEvery   = 5 * time.Second
)

// IssueCert performs the BYOD cert flow for a single service:
//
//  1. Find the ContainerApp by tag (defang-service: <serviceName>) in the
//     project's resource group.
//  2. Wait for DNS records (CNAME -> app FQDN, TXT asuid.<host> -> verificationId).
//  3. Register the custom hostname with bindingType: Disabled (validates asuid TXT).
//  4. Issue a managed certificate via CNAME validation.
//  5. Flip the customDomain to bindingType: SniEnabled, attaching the cert.
//  6. Verify TLS is serving on https://<hostname>/.
//
// Each ARM step is idempotent: re-running after a partial failure picks up
// where it left off.
func (b *ByocAzure) IssueCert(ctx context.Context, projectName, serviceName, hostname string) error {
	cred, err := b.driver.NewCreds()
	if err != nil {
		return err
	}
	appsClient, err := armappcontainers.NewContainerAppsClient(b.driver.SubscriptionID, cred, nil)
	if err != nil {
		return fmt.Errorf("creating container apps client: %w", err)
	}
	certsClient, err := armappcontainers.NewManagedCertificatesClient(b.driver.SubscriptionID, cred, nil)
	if err != nil {
		return fmt.Errorf("creating managed certificates client: %w", err)
	}

	rg := b.projectResourceGroupName(projectName)
	app, err := findContainerAppByService(ctx, appsClient, rg, serviceName)
	if err != nil {
		return err
	}
	appName := derefString(app.Name)
	if app.Properties == nil ||
		app.Properties.CustomDomainVerificationID == nil ||
		app.Properties.Configuration == nil ||
		app.Properties.Configuration.Ingress == nil ||
		app.Properties.Configuration.Ingress.Fqdn == nil ||
		app.Properties.ManagedEnvironmentID == nil {
		return fmt.Errorf("container app %q is missing required ingress/verificationId fields", appName)
	}
	vid := *app.Properties.CustomDomainVerificationID
	appFqdn := *app.Properties.Configuration.Ingress.Fqdn
	envID := *app.Properties.ManagedEnvironmentID
	envName := envID[strings.LastIndex(envID, "/")+1:]

	if err := waitForBYODdns(ctx, hostname, appFqdn, vid); err != nil {
		return err
	}

	term.Infof("Registering custom hostname %s on container app %s", hostname, appName)
	if err := addHostnameDisabled(ctx, appsClient, rg, appName, hostname, app); err != nil {
		return err
	}

	certName := managedCertName(envName, hostname)
	term.Infof("Issuing managed certificate %s (this may take up to ~5 minutes)", certName)
	issued, err := issueManagedCertificate(ctx, certsClient, rg, envName, certName, hostname)
	if err != nil {
		return err
	}

	term.Infof("Binding cert to %s on %s", hostname, appName)
	if err := bindHostnameSniEnabled(ctx, appsClient, rg, appName, hostname, derefString(issued.ID)); err != nil {
		return err
	}

	term.Infof("Waiting for TLS to come online on https://%s/", hostname)
	return waitForTLS(ctx, hostname)
}

// findContainerAppByService lists ContainerApps in rg and returns the one
// whose tag map contains defang-service: serviceName.
func findContainerAppByService(ctx context.Context, client *armappcontainers.ContainerAppsClient, rg, serviceName string) (*armappcontainers.ContainerApp, error) {
	pager := client.NewListByResourceGroupPager(rg, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("listing container apps in %s: %w", rg, err)
		}
		for _, app := range page.Value {
			if app == nil || app.Tags == nil {
				continue
			}
			if v, ok := app.Tags[serviceTagKey]; ok && v != nil && *v == serviceName {
				return app, nil
			}
		}
	}
	return nil, fmt.Errorf("no Container App in %s tagged %s=%s", rg, serviceTagKey, serviceName)
}

// waitForBYODdns blocks until both the CNAME and asuid TXT records resolve
// correctly, prompting the user once with the values to add.
func waitForBYODdns(ctx context.Context, hostname, expectedCname, expectedTxt string) error {
	asuid := "asuid." + hostname
	deadline := time.Now().Add(dnsWaitTimeout)
	promptShown := false

	for {
		cnameOK := dns.CheckDomainDNSReady(ctx, hostname, []string{expectedCname})
		txtOK, _ := lookupTXTContains(ctx, asuid, expectedTxt) // FIXME: DNS resolver should be able to lookup TXT records
		if cnameOK && txtOK {
			term.Infof("DNS records for %s verified", hostname)
			return nil
		}
		if !promptShown {
			term.Printf("Configure DNS records for %s:\n", hostname)
			term.Printf("  CNAME  %s              ->  %s\n", hostname, expectedCname)
			term.Printf("  TXT    asuid.%s        ->  %s\n", hostname, expectedTxt)
			term.Infof("Waiting for DNS propagation (timeout %v)...", dnsWaitTimeout)
			promptShown = true
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out after %v waiting for DNS records on %s", dnsWaitTimeout, hostname)
		}
		if err := pkg.SleepWithContext(ctx, dnsPollEvery); err != nil {
			return err
		}
	}
}

func lookupTXTContains(ctx context.Context, name, expected string) (bool, error) {
	txts, err := net.DefaultResolver.LookupTXT(ctx, name)
	if err != nil {
		return false, err
	}
	for _, t := range txts {
		if t == expected {
			return true, nil
		}
	}
	return false, nil
}

// addHostnameDisabled PATCHes the ContainerApp to add (or no-op) a customDomain
// entry with bindingType: Disabled. Disabled doesn't require a cert, but does
// validate asuid TXT — that's why we wait for DNS first.
func addHostnameDisabled(ctx context.Context, client *armappcontainers.ContainerAppsClient, rg, appName, hostname string, current *armappcontainers.ContainerApp) error {
	if hasCustomDomain(current, hostname) {
		term.Debugf("Hostname %s already registered on %s", hostname, appName)
		return nil
	}
	body := armappcontainers.ContainerApp{
		Properties: &armappcontainers.ContainerAppProperties{
			Configuration: &armappcontainers.Configuration{
				Ingress: &armappcontainers.Ingress{
					CustomDomains: []*armappcontainers.CustomDomain{{
						Name:        to.Ptr(hostname),
						BindingType: to.Ptr(armappcontainers.BindingTypeDisabled),
					}},
				},
			},
		},
	}
	poller, err := client.BeginUpdate(ctx, rg, appName, body, nil)
	if err != nil {
		return fmt.Errorf("registering hostname %s: %w", hostname, err)
	}
	if _, err := poller.PollUntilDone(ctx, nil); err != nil {
		return fmt.Errorf("registering hostname %s: %w", hostname, err)
	}
	return nil
}

func hasCustomDomain(app *armappcontainers.ContainerApp, hostname string) bool {
	if app == nil || app.Properties == nil ||
		app.Properties.Configuration == nil ||
		app.Properties.Configuration.Ingress == nil {
		return false
	}
	for _, cd := range app.Properties.Configuration.Ingress.CustomDomains {
		if cd != nil && cd.Name != nil && *cd.Name == hostname {
			return true
		}
	}
	return false
}

// issueManagedCertificate creates the managed cert via CNAME validation. ARM
// requires the hostname to already be registered as a customDomain on some app
// in the env; that's why addHostnameDisabled runs first.
func issueManagedCertificate(ctx context.Context, client *armappcontainers.ManagedCertificatesClient, rg, envName, certName, hostname string) (*armappcontainers.ManagedCertificate, error) {
	envelope := armappcontainers.ManagedCertificate{
		Properties: &armappcontainers.ManagedCertificateProperties{
			SubjectName:             to.Ptr(hostname),
			DomainControlValidation: to.Ptr(armappcontainers.ManagedCertificateDomainControlValidationCNAME),
		},
	}
	poller, err := client.BeginCreateOrUpdate(ctx, rg, envName, certName, &armappcontainers.ManagedCertificatesClientBeginCreateOrUpdateOptions{
		ManagedCertificateEnvelope: &envelope,
	})
	if err != nil {
		return nil, fmt.Errorf("issuing managed certificate %s: %w", certName, err)
	}
	resp, err := poller.PollUntilDone(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("issuing managed certificate %s: %w", certName, err)
	}
	return &resp.ManagedCertificate, nil
}

// bindHostnameSniEnabled PATCHes customDomain entry to bindingType: SniEnabled
// with the issued cert ID. After this, https://<hostname>/ serves the cert.
func bindHostnameSniEnabled(ctx context.Context, client *armappcontainers.ContainerAppsClient, rg, appName, hostname, certID string) error {
	body := armappcontainers.ContainerApp{
		Properties: &armappcontainers.ContainerAppProperties{
			Configuration: &armappcontainers.Configuration{
				Ingress: &armappcontainers.Ingress{
					CustomDomains: []*armappcontainers.CustomDomain{{
						Name:          to.Ptr(hostname),
						BindingType:   to.Ptr(armappcontainers.BindingTypeSniEnabled),
						CertificateID: to.Ptr(certID),
					}},
				},
			},
		},
	}
	poller, err := client.BeginUpdate(ctx, rg, appName, body, nil)
	if err != nil {
		return fmt.Errorf("binding %s on %s: %w", hostname, appName, err)
	}
	if _, err := poller.PollUntilDone(ctx, nil); err != nil {
		return fmt.Errorf("binding %s on %s: %w", hostname, appName, err)
	}
	return nil
}

func waitForTLS(ctx context.Context, hostname string) error {
	deadline := time.Now().Add(tlsWaitTimeout)
	for {
		if err := cert.CheckTLSCert(ctx, hostname); err == nil {
			term.Infof("TLS cert for %s is online", hostname)
			return nil
		}
		if time.Now().After(deadline) {
			return errors.New("timed out waiting for TLS certificate to come online")
		}
		if err := pkg.SleepWithContext(ctx, tlsPollEvery); err != nil {
			return err
		}
	}
}

// managedCertName builds an ARM-safe managed-certificate resource name.
// ARM allows alphanumeric + hyphens, max 64 chars; we keep it well under that.
func managedCertName(envName, hostname string) string {
	env := sanitize(envName)
	if len(env) > 15 {
		env = env[:15]
	}
	host := sanitize(hostname)
	if len(host) > 30 {
		host = host[:30]
	}
	return fmt.Sprintf("mc-%s-%s", env, host)
}

func sanitize(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
