package azure

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
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
func (b *ByocAzure) IssueCert(ctx context.Context, projectName, serviceName, hostname string, resolverAt func(string) dns.Resolver) error {
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

	if err := waitForBYODdns(ctx, hostname, appFqdn, vid, resolverAt); err != nil {
		return err
	}

	term.Infof("Registering custom hostname %s on container app %s", hostname, appName)
	if err := addHostnameDisabled(ctx, appsClient, rg, appName, hostname, app); err != nil {
		return err
	}

	certName := managedCertName(envName, hostname)
	term.Infof("Issuing managed certificate %s (this may take up to ~5 minutes)", certName)
	issued, err := issueManagedCertificate(ctx, certsClient, rg, envName, certName, hostname, derefString(app.Location))
	if err != nil {
		return err
	}

	term.Infof("Binding cert to %s on %s", hostname, appName)
	if err := bindHostnameSniEnabled(ctx, appsClient, rg, appName, hostname, derefString(issued.ID)); err != nil {
		return err
	}

	term.Infof("Waiting for TLS to come online on https://%s/", hostname)
	return waitForTLS(ctx, hostname, resolverAt(""))
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
func waitForBYODdns(ctx context.Context, hostname, expectedCname, expectedTxt string, resolverAt func(string) dns.Resolver) error {
	asuid := "asuid." + hostname
	deadline := time.Now().Add(dnsWaitTimeout)
	promptShown := false

	for {
		cnameOK := dns.CheckDomainDNSReady(ctx, hostname, []string{expectedCname}, resolverAt)
		txtOK, _ := dns.LookupTXTContains(ctx, asuid, expectedTxt, resolverAt(""))
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

// addHostnameDisabled PATCHes the ContainerApp to add (or no-op) a customDomain
// entry with bindingType: Disabled. Disabled doesn't require a cert, but does
// validate asuid TXT — that's why we wait for DNS first.
//
// Azure ARM uses JSON Merge Patch (RFC 7396) which replaces arrays wholesale,
// so we must include every existing CustomDomain entry in the body or they
// will be wiped out by the update.
func addHostnameDisabled(ctx context.Context, client *armappcontainers.ContainerAppsClient, rg, appName, hostname string, current *armappcontainers.ContainerApp) error {
	if hasCustomDomain(current, hostname) {
		term.Debugf("Hostname %s already registered on %s", hostname, appName)
		return nil
	}
	domains := append(existingCustomDomains(current), &armappcontainers.CustomDomain{
		Name:        to.Ptr(hostname),
		BindingType: to.Ptr(armappcontainers.BindingTypeDisabled),
	})
	body := armappcontainers.ContainerApp{
		Properties: &armappcontainers.ContainerAppProperties{
			Configuration: &armappcontainers.Configuration{
				Ingress: &armappcontainers.Ingress{
					CustomDomains: domains,
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

// existingCustomDomains returns the current CustomDomains slice from a
// ContainerApp, navigating the optional pointer chain safely.
func existingCustomDomains(app *armappcontainers.ContainerApp) []*armappcontainers.CustomDomain {
	if app == nil || app.Properties == nil ||
		app.Properties.Configuration == nil ||
		app.Properties.Configuration.Ingress == nil {
		return nil
	}
	return app.Properties.Configuration.Ingress.CustomDomains
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
// issueManagedCertificate creates the managed cert. CNAME validation is the
// default and works for any hostname that has a CNAME (subdomains pointing at
// the Container Apps FQDN). Apex domains can't have a CNAME (RFC 1034) and
// Azure rejects CNAME validation with InvalidValidationMethod; in that case
// we fall back to TXT validation, which requires the user to add a
// _dnsauth.<hostname> TXT record with the validationToken Azure returns.
func issueManagedCertificate(ctx context.Context, client *armappcontainers.ManagedCertificatesClient, rg, envName, certName, hostname, location string) (*armappcontainers.ManagedCertificate, error) {
	resp, err := submitManagedCert(ctx, client, rg, envName, certName, hostname, location, armappcontainers.ManagedCertificateDomainControlValidationCNAME)
	if err == nil {
		return resp, nil
	}
	if !isInvalidValidationMethod(err) {
		return nil, fmt.Errorf("issuing managed certificate %s: %w", certName, err)
	}
	term.Infof("CNAME validation rejected for %s (apex domain); falling back to TXT validation", hostname)
	return submitManagedCertTXT(ctx, client, rg, envName, certName, hostname, location)
}

// submitManagedCert creates the cert with the given validation method and
// waits for the poller to complete. CNAME validation completes synchronously
// once Azure verifies the existing CNAME record on the hostname.
func submitManagedCert(ctx context.Context, client *armappcontainers.ManagedCertificatesClient, rg, envName, certName, hostname, location string, method armappcontainers.ManagedCertificateDomainControlValidation) (*armappcontainers.ManagedCertificate, error) {
	envelope := armappcontainers.ManagedCertificate{
		// Required by ARM — must match the managed environment's region or
		// BeginCreateOrUpdate fails with "LocationRequired".
		Location: to.Ptr(location),
		Properties: &armappcontainers.ManagedCertificateProperties{
			SubjectName:             to.Ptr(hostname),
			DomainControlValidation: to.Ptr(method),
		},
	}
	poller, err := client.BeginCreateOrUpdate(ctx, rg, envName, certName, &armappcontainers.ManagedCertificatesClientBeginCreateOrUpdateOptions{
		ManagedCertificateEnvelope: &envelope,
	})
	if err != nil {
		return nil, err
	}
	pollResp, err := poller.PollUntilDone(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &pollResp.ManagedCertificate, nil
}

// submitManagedCertTXT creates the cert with TXT validation and walks the
// user through the dnsauth.<hostname> DNS record dance. Unlike CNAME, the
// initial PUT response includes a validationToken that we must surface
// before Azure can complete validation; we GET the cert in a loop until the
// token is populated, prompt once, then wait for ProvisioningState=Succeeded.
func submitManagedCertTXT(ctx context.Context, client *armappcontainers.ManagedCertificatesClient, rg, envName, certName, hostname, location string) (*armappcontainers.ManagedCertificate, error) {
	envelope := armappcontainers.ManagedCertificate{
		Location: to.Ptr(location),
		Properties: &armappcontainers.ManagedCertificateProperties{
			SubjectName:             to.Ptr(hostname),
			DomainControlValidation: to.Ptr(armappcontainers.ManagedCertificateDomainControlValidationTXT),
		},
	}
	poller, err := client.BeginCreateOrUpdate(ctx, rg, envName, certName, &armappcontainers.ManagedCertificatesClientBeginCreateOrUpdateOptions{
		ManagedCertificateEnvelope: &envelope,
	})
	if err != nil {
		return nil, fmt.Errorf("issuing managed certificate %s (TXT): %w", certName, err)
	}

	// Poll GETs in parallel with the long-running PUT to fetch the token as
	// soon as Azure populates it. Azure typically sets ValidationToken within
	// the first few seconds after the PUT; the long-running operation only
	// completes once the user adds the matching dnsauth TXT record.
	tokenDeadline := time.Now().Add(5 * time.Minute)
	var token string
	for token == "" {
		got, getErr := client.Get(ctx, rg, envName, certName, nil)
		if getErr == nil && got.Properties != nil && got.Properties.ValidationToken != nil {
			token = *got.Properties.ValidationToken
			break
		}
		if time.Now().After(tokenDeadline) {
			return nil, fmt.Errorf("timed out waiting for Azure to issue validationToken for %s", hostname)
		}
		if err := pkg.SleepWithContext(ctx, 5*time.Second); err != nil {
			return nil, err
		}
	}

	dnsauth := "_dnsauth." + hostname
	term.Printf("Add TXT record for managed cert validation:\n")
	term.Printf("  TXT  %s  ->  %s\n", dnsauth, token)
	term.Infof("Waiting for %s and cert provisioning to finish (timeout ~5m)...", dnsauth)

	pollResp, err := poller.PollUntilDone(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("issuing managed certificate %s (TXT): %w", certName, err)
	}
	return &pollResp.ManagedCertificate, nil
}

// isInvalidValidationMethod returns true when Azure rejected the requested
// validation method — this is what happens for apex domains where CNAME
// isn't a valid validation method. Detected via ARM's ErrorCode field
// (top-level), with a string fallback for safety.
func isInvalidValidationMethod(err error) bool {
	if err == nil {
		return false
	}
	var respErr *azcore.ResponseError
	if errors.As(err, &respErr) && respErr.ErrorCode == "InvalidValidationMethod" {
		return true
	}
	return strings.Contains(err.Error(), "InvalidValidationMethod")
}

// bindHostnameSniEnabled PATCHes the customDomain entry to bindingType:
// SniEnabled with the issued cert ID. After this, https://<hostname>/ serves
// the cert.
//
// Azure ARM uses JSON Merge Patch (RFC 7396) which replaces arrays wholesale,
// so we fetch the current state, update the matching entry in place, and send
// the full CustomDomains array back. Otherwise every other custom domain on
// the app would be dropped by the PATCH.
func bindHostnameSniEnabled(ctx context.Context, client *armappcontainers.ContainerAppsClient, rg, appName, hostname, certID string) error {
	cur, err := client.Get(ctx, rg, appName, nil)
	if err != nil {
		return fmt.Errorf("fetching app %s before cert bind: %w", appName, err)
	}
	domains := existingCustomDomains(&cur.ContainerApp)
	updated := false
	for _, cd := range domains {
		if cd != nil && cd.Name != nil && *cd.Name == hostname {
			cd.BindingType = to.Ptr(armappcontainers.BindingTypeSniEnabled)
			cd.CertificateID = to.Ptr(certID)
			updated = true
		}
	}
	if !updated {
		domains = append(domains, &armappcontainers.CustomDomain{
			Name:          to.Ptr(hostname),
			BindingType:   to.Ptr(armappcontainers.BindingTypeSniEnabled),
			CertificateID: to.Ptr(certID),
		})
	}
	body := armappcontainers.ContainerApp{
		Properties: &armappcontainers.ContainerAppProperties{
			Configuration: &armappcontainers.Configuration{
				Ingress: &armappcontainers.Ingress{
					CustomDomains: domains,
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

func waitForTLS(ctx context.Context, hostname string, resolver dns.Resolver) error {
	deadline := time.Now().Add(tlsWaitTimeout)
	for {
		if err := cert.CheckTLSCert(ctx, hostname, resolver); err == nil {
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
//
// We trim trailing hyphens after truncation so the joined name never produces
// a "--" run, which ARM rejects.
func managedCertName(envName, hostname string) string {
	env := sanitize(envName)
	if len(env) > 15 {
		env = strings.TrimRight(env[:15], "-")
	}
	host := sanitize(hostname)
	if len(host) > 30 {
		host = strings.TrimRight(host[:30], "-")
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
