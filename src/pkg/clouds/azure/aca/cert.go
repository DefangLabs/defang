// Managed-certificate provisioning for Azure Container Apps. Lives in the
// cloud-SDK layer (pkg/clouds/azure/aca) so both the defang CLI and the CD
// task can call it without one layer reaching across the other — keeping the
// CLI's `defang cert generate` flow and the CD task's post-deploy auto-cert
// step on a single code path.

package aca

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
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
	// ServiceTagKey matches the Pulumi provider's ServiceTags map key
	// (provider/defangazure/azure/azure.go: ServiceTags). Used to look up
	// the ContainerApp belonging to a Compose service without depending on
	// a deterministic name format. Exported so callers that need to filter
	// CAs by service can use the same key.
	ServiceTagKey = "defang-service"

	dnsWaitTimeout = 30 * time.Minute
	dnsPollEvery   = 15 * time.Second
	tlsWaitTimeout = 10 * time.Minute
	tlsPollEvery   = 5 * time.Second
)

// certTarget is the ContainerApp-derived state both PreflightCert and
// IssueCert need: which app to patch, the ownership token to publish, and the
// FQDN / environment the routing record and managed cert hang off.
type certTarget struct {
	appsClient  *armappcontainers.ContainerAppsClient
	certsClient *armappcontainers.ManagedCertificatesClient
	envsClient  *armappcontainers.ManagedEnvironmentsClient

	resourceGroup string
	app           *armappcontainers.ContainerApp
	appName       string
	vid           string // CustomDomainVerificationID, published as the asuid TXT value
	appFqdn       string
	envName       string
	certName      string
}

// resolveCertTarget locates the ContainerApp tagged for serviceName and derives
// everything the cert flow needs from it.
//
// PreflightCert and IssueCert each call this rather than one passing an opaque
// handle to the other: the two run in separate phases of a provider-generic CLI
// flow (see cli/cert.go runIssuerJobs), and threading Azure state through that
// flow would leak ARM types into it. The cost is one extra ContainerApps list
// per hostname, which is negligible next to the DNS waits and the two
// long-running ARM operations that follow.
func resolveCertTarget(ctx context.Context, cred azcore.TokenCredential, subscriptionID, resourceGroup, serviceName, hostname string) (*certTarget, error) {
	appsClient, err := armappcontainers.NewContainerAppsClient(subscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("creating container apps client: %w", err)
	}
	certsClient, err := armappcontainers.NewManagedCertificatesClient(subscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("creating managed certificates client: %w", err)
	}
	envsClient, err := armappcontainers.NewManagedEnvironmentsClient(subscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("creating managed environments client: %w", err)
	}

	app, err := findContainerAppByService(ctx, appsClient, resourceGroup, serviceName)
	if err != nil {
		return nil, err
	}
	appName := derefString(app.Name)
	if app.Properties == nil ||
		app.Properties.CustomDomainVerificationID == nil ||
		app.Properties.Configuration == nil ||
		app.Properties.Configuration.Ingress == nil ||
		app.Properties.Configuration.Ingress.Fqdn == nil ||
		app.Properties.ManagedEnvironmentID == nil {
		return nil, fmt.Errorf("container app %q is missing required ingress/verificationId fields", appName)
	}
	envID := *app.Properties.ManagedEnvironmentID
	envName := envID[strings.LastIndex(envID, "/")+1:]
	return &certTarget{
		appsClient:    appsClient,
		certsClient:   certsClient,
		envsClient:    envsClient,
		resourceGroup: resourceGroup,
		app:           app,
		appName:       appName,
		vid:           *app.Properties.CustomDomainVerificationID,
		appFqdn:       *app.Properties.Configuration.Ingress.Fqdn,
		envName:       envName,
		certName:      managedCertName(envName, hostname),
	}, nil
}

// PreflightCert reports the DNS records `hostname` still needs before a cert
// can be issued for it, doing no provisioning work of its own. A nil result
// means there is nothing for the user to do: either both records are already
// live, or the hostname is already serving TLS.
//
// This exists so the caller can probe every hostname first and present all
// pending records in one block, before any hostname starts waiting — a user
// staring at their DNS dashboard wants the full list in one sitting, not one
// domain's records at a time as each worker happens to reach them. IssueCert
// therefore does *not* print the records; callers that want them shown must
// run this first.
func PreflightCert(ctx context.Context, cred azcore.TokenCredential, subscriptionID, resourceGroup, serviceName, hostname string, resolverAt func(string) dns.Resolver) ([]dns.RequiredRecord, error) {
	target, err := resolveCertTarget(ctx, cred, subscriptionID, resourceGroup, serviceName, hostname)
	if err != nil {
		return nil, err
	}
	if alreadyServingTLS(ctx, target.certsClient, resourceGroup, target.envName, target.certName, target.app, hostname) {
		return nil, nil
	}
	return target.pendingRecords(ctx, hostname, resolverAt), nil
}

// pendingRecords returns both records the hostname needs whenever either is
// missing: TXT and the routing record are waited on separately, but the user
// wants to add everything in one sitting rather than being told about the
// routing record only after TXT has already propagated (lionello, PR review
// on #2222).
//
// Only one of CNAME/A ever applies to a given hostname — showing both, as if
// either could be added, misled the user into thinking they had a choice
// (lionello, PR review on #2222, r3816456453). dns.IsApexDomain is a static,
// DNS-lookup-free check on the name itself, so the right record can be picked
// before any DNS is configured; it's independent of the live-DNS apex detection
// dns.CheckDomainDNSReady and the managed-cert validation-method fallback use
// once records exist.
func (t *certTarget) pendingRecords(ctx context.Context, hostname string, resolverAt func(string) dns.Resolver) []dns.RequiredRecord {
	txtOK, _ := dns.LookupTXTContains(ctx, "asuid."+hostname, t.vid, resolverAt(""))
	routeOK := dns.CheckDomainDNSReady(ctx, hostname, []string{t.appFqdn}, resolverAt)
	if txtOK && routeOK {
		return nil
	}
	records := []dns.RequiredRecord{{
		Type:  "TXT",
		Name:  "asuid." + hostname,
		Value: t.vid,
		Note:  "add this first — the hostname can register as soon as this is live",
	}}
	if dns.IsApexDomain(hostname) {
		records = append(records, dns.RequiredRecord{
			Type:  "A",
			Name:  hostname,
			Value: fetchEnvironmentStaticIP(ctx, t.envsClient, t.resourceGroup, t.envName),
			Note:  "needed before the cert can be issued",
		})
	} else {
		records = append(records, dns.RequiredRecord{
			Type:  "CNAME",
			Name:  hostname,
			Value: t.appFqdn,
			Note:  "needed before the cert can be issued",
		})
	}
	return records
}

// IssueCert provisions a TLS cert for `hostname` on the ContainerApp tagged
// for `serviceName` in `resourceGroup`. Steps:
//
//  1. Find the ContainerApp by tag (defang-service: <serviceName>).
//  2. Wait for TXT asuid.<host> → verificationId (ownership proof only; no
//     routing record needed for this step).
//  3. Register the custom hostname with bindingType: Disabled (validates asuid TXT).
//  4. Wait for the routing record: subdomain → CNAME to app FQDN, apex → A
//     record to the env IP. Only needed now, ahead of cert issuance/binding —
//     registering the hostname (steps 2-3) does not require live routing, so a
//     caller that adds the asuid TXT record ahead of a DNS cutover lets Azure
//     start validating ownership immediately instead of blocking on both
//     records together.
//  5. Issue a managed certificate (subdomain → CNAME validation; apex → HTTP).
//  6. Flip the customDomain to bindingType: SniEnabled, attaching the cert.
//  7. Verify TLS is serving on https://<hostname>/.
//
// Apex domains (e.g. example.com) can't have a CNAME (RFC 1034), so they route
// via an A record and validate over HTTP. The wait/validation logic confirms
// apex-ness from live DNS and from Azure's validation-method rejection — no
// caller hint is needed.
//
// The records the user must create are *not* printed here; call PreflightCert
// for those, so a caller issuing for several hostnames can show them all in one
// block before any of them starts waiting.
//
// Every user-facing progress line goes through log, which the CLI points at a
// per-domain prefixed logger so concurrent hostnames stay readable; pass nil to
// get plain term.Infof output.
//
// Each ARM step is idempotent: re-running after a partial failure picks up
// where it left off. resolverAt is used by steps 2 and 4 to chase the DNS chain
// — pass dns.DirectResolverAt to query authoritative servers directly (CD task)
// or dns.NewFabricResolverAt(client) to route through Fabric (CLI).
//
// Steps 3 and 6 read-modify-write the ContainerApp's CustomDomains array via
// ARM's JSON Merge Patch, which replaces the array wholesale. A service can
// have multiple hostnames (e.g. an apex domain plus a www alias) processed as
// concurrent domainJobs (see cli/cert.go runIssuerJobs) that all target the
// same ContainerApp — without serialization, two concurrent read-modify-writes
// race: whichever PATCH lands last, built from a Get that predates the other's
// write, silently drops the other's just-added binding. appLock guards exactly
// that critical section, per (resourceGroup, appName), while leaving DNS waits
// and cert issuance (independent per hostname) fully parallel.
func IssueCert(ctx context.Context, cred azcore.TokenCredential, subscriptionID, resourceGroup, serviceName, hostname string, resolverAt func(string) dns.Resolver, log func(string, ...any)) error {
	if log == nil {
		log = func(format string, args ...any) { term.Infof(format, args...) }
	}

	target, err := resolveCertTarget(ctx, cred, subscriptionID, resourceGroup, serviceName, hostname)
	if err != nil {
		return err
	}

	// Short-circuit: if the hostname is already bound SniEnabled with a cert and
	// that managed cert is in the Succeeded state, the host is already serving
	// TLS — skip the DNS wait, the cert create-or-update, and the SNI re-bind.
	// All of those are idempotent but run two long-running ARM operations per
	// service on every deploy; this avoids that work when nothing changed.
	if alreadyServingTLS(ctx, target.certsClient, resourceGroup, target.envName, target.certName, target.app, hostname) {
		term.Debugf("Cert %s for %s already provisioned and bound; skipping issuance", target.certName, hostname)
		return nil
	}

	deadline := time.Now().Add(dnsWaitTimeout)
	if err := waitForAsuidTXT(ctx, hostname, target.vid, deadline, resolverAt, log); err != nil {
		return err
	}

	log("registering custom hostname on container app %s…", target.appName)
	if err := addHostnameDisabled(ctx, target.appsClient, resourceGroup, target.appName, hostname); err != nil {
		return err
	}

	if err := waitForRoutingRecord(ctx, hostname, target.appFqdn, deadline, resolverAt, log); err != nil {
		return err
	}

	log("issuing managed certificate %s (may take up to ~5 minutes)…", target.certName)
	issued, err := issueManagedCertificate(ctx, target.certsClient, resourceGroup, target.envName, target.certName, hostname, derefString(target.app.Location), log)
	if err != nil {
		return err
	}

	log("binding cert on container app %s…", target.appName)
	if err := bindHostnameSniEnabled(ctx, target.appsClient, resourceGroup, target.appName, hostname, derefString(issued.ID)); err != nil {
		return err
	}

	log("waiting for TLS cert to come online…")
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
			if v, ok := app.Tags[ServiceTagKey]; ok && v != nil && *v == serviceName {
				return app, nil
			}
		}
	}
	return nil, fmt.Errorf("no Container App in %s tagged %s=%s", rg, ServiceTagKey, serviceName)
}

// waitForAsuidTXT blocks until the asuid.<hostname> TXT record (Azure's
// domain-ownership proof) resolves. Unlike the routing record, this is
// required only for hostname registration (addHostnameDisabled) — it does
// not need traffic to route to Azure yet, so a caller can add just this
// record ahead of a DNS cutover and have the hostname registered/verified in
// advance. The record values themselves are printed once by the caller's
// pre-flight, covering both this and the routing record — see PreflightCert.
func waitForAsuidTXT(ctx context.Context, hostname, expectedTxt string, deadline time.Time, resolverAt func(string) dns.Resolver, log func(string, ...any)) error {
	asuid := "asuid." + hostname
	waitingLogged := false

	for {
		if txtOK, _ := dns.LookupTXTContains(ctx, asuid, expectedTxt, resolverAt("")); txtOK {
			return nil
		}
		if !waitingLogged {
			log("waiting for the asuid TXT record to propagate (timeout %v)…", time.Until(deadline).Round(time.Second))
			waitingLogged = true
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out after %v waiting for the asuid TXT record on %s", dnsWaitTimeout, hostname)
		}
		if err := pkg.SleepWithContext(ctx, dnsPollEvery); err != nil {
			return err
		}
	}
}

// waitForRoutingRecord blocks until the routing record resolves. Required
// before cert issuance: CNAME validation needs a live CNAME, and HTTP
// validation needs live routing to reach the app for the challenge.
//
// The routing record is a CNAME → app FQDN for a subdomain, or an A record for
// an apex domain (no CNAME possible at the zone apex). Both are covered by
// dns.CheckDomainDNSReady: it accepts a CNAME to expectedCname, or an A record
// whose addresses match those of expectedCname — which for Container Apps is the
// managed environment's static IP, exactly what an apex A record must point at.
// So an apex domain is validated against the intended target, not merely
// "resolves to something". The record values themselves are printed once by
// the caller's pre-flight, covering both this and the TXT record — see
// PreflightCert.
func waitForRoutingRecord(ctx context.Context, hostname, expectedCname string, deadline time.Time, resolverAt func(string) dns.Resolver, log func(string, ...any)) error {
	waitingLogged := false

	for {
		if dns.CheckDomainDNSReady(ctx, hostname, []string{expectedCname}, resolverAt) {
			log("DNS verified")
			return nil
		}
		if !waitingLogged {
			log("waiting for the routing record to propagate (timeout %v)…", time.Until(deadline).Round(time.Second))
			waitingLogged = true
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out after %v waiting for the routing record on %s", dnsWaitTimeout, hostname)
		}
		if err := pkg.SleepWithContext(ctx, dnsPollEvery); err != nil {
			return err
		}
	}
}

// fetchEnvironmentStaticIP fetches the Container Apps environment's inbound
// static IP over ARM for display in the apex A-record instructions. This is a
// JIT lookup done only once the DNS-not-ready prompt is about to be shown, so
// the common case (DNS already configured) never pays for it. A lookup
// failure, or an empty StaticIP (e.g. an environment still provisioning),
// falls back to a placeholder rather than failing cert issuance — the value
// is purely informational; dns.CheckDomainDNSReady validates the actual DNS
// state itself.
func fetchEnvironmentStaticIP(ctx context.Context, envsClient *armappcontainers.ManagedEnvironmentsClient, resourceGroup, envName string) string {
	env, err := envsClient.Get(ctx, resourceGroup, envName, nil)
	if err != nil || env.Properties == nil || env.Properties.StaticIP == nil || *env.Properties.StaticIP == "" {
		term.Debugf("Could not fetch static IP for environment %s: %v", envName, err)
		return "<Container Apps environment IP; check the Azure portal>"
	}
	return *env.Properties.StaticIP
}

// appLocks serializes the read-modify-write critical section in
// addHostnameDisabled and bindHostnameSniEnabled per (resourceGroup, appName).
// A single service can have multiple hostnames (e.g. an apex domain plus a
// www alias) processed as concurrent domainJobs that all target the same
// ContainerApp; without this, two concurrent Get-modify-PATCH sequences race
// and the later PATCH — built from a Get that predates the other's write —
// silently drops the other's just-added customDomain entry.
var appLocks sync.Map // map[string]*sync.Mutex, keyed by rg+"/"+appName

func lockForApp(rg, appName string) *sync.Mutex {
	v, _ := appLocks.LoadOrStore(rg+"/"+appName, &sync.Mutex{})
	mu, ok := v.(*sync.Mutex)
	if !ok {
		panic("appLocks: stored value is not a *sync.Mutex") // unreachable: only ever stores *sync.Mutex
	}
	return mu
}

// addHostnameDisabled PATCHes the ContainerApp to add (or no-op) a customDomain
// entry with bindingType: Disabled. Disabled doesn't require a cert, but does
// validate asuid TXT — that's why we wait for DNS first.
//
// Azure ARM uses JSON Merge Patch (RFC 7396) which replaces arrays wholesale,
// so we must include every existing CustomDomain entry in the body or they
// will be wiped out by the update. We re-Get the CA right before the PATCH
// rather than reusing a snapshot from the caller: the DNS wait can block for
// up to 30 minutes, during which another deploy could have added its own
// customDomain entry, and a stale slice would silently drop that. The
// Get-through-PATCH sequence is itself guarded by appLock so a concurrent
// call for a sibling hostname on the same app can't interleave its own
// Get-modify-PATCH in between and clobber this one.
func addHostnameDisabled(ctx context.Context, client *armappcontainers.ContainerAppsClient, rg, appName, hostname string) error {
	mu := lockForApp(rg, appName)
	mu.Lock()
	defer mu.Unlock()

	cur, err := client.Get(ctx, rg, appName, nil)
	if err != nil {
		return fmt.Errorf("fetching app %s before hostname registration: %w", appName, err)
	}
	current := &cur.ContainerApp
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

// alreadyServingTLS reports whether hostname is already fully provisioned on
// the app: registered as a custom domain bound SniEnabled with a non-empty
// certificate ID, AND the managed cert `certName` exists in the environment
// with ProvisioningState == Succeeded. When true, IssueCert has nothing to do.
//
// The managed cert GET is the only network call; it's a cheap read versus the
// two long-running create-or-update/bind operations it lets us skip. A GET
// error (e.g. cert not found) is treated as "not provisioned" so issuance
// proceeds normally.
func alreadyServingTLS(ctx context.Context, certsClient *armappcontainers.ManagedCertificatesClient, rg, envName, certName string, app *armappcontainers.ContainerApp, hostname string) bool {
	if !hostnameBoundWithCert(app, hostname) {
		return false
	}
	got, err := certsClient.Get(ctx, rg, envName, certName, nil)
	if err != nil {
		return false
	}
	return got.Properties != nil &&
		got.Properties.ProvisioningState != nil &&
		*got.Properties.ProvisioningState == armappcontainers.CertificateProvisioningStateSucceeded
}

// hostnameBoundWithCert reports whether the app already has hostname registered
// as a custom domain bound SniEnabled with a non-empty certificate ID.
func hostnameBoundWithCert(app *armappcontainers.ContainerApp, hostname string) bool {
	for _, cd := range existingCustomDomains(app) {
		if cd != nil && cd.Name != nil && *cd.Name == hostname {
			return cd.BindingType != nil &&
				*cd.BindingType == armappcontainers.BindingTypeSniEnabled &&
				cd.CertificateID != nil && *cd.CertificateID != ""
		}
	}
	return false
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

// issueManagedCertificate creates the managed cert, choosing the validation
// method to match the hostname's routing record. CNAME validation is the
// default and works for subdomains pointing at the Container Apps FQDN. Apex
// domains can't have a CNAME (RFC 1034), so Azure rejects CNAME validation with
// InvalidValidationMethod — we detect that and retry with HTTP validation, which
// Container Apps completes automatically once the apex A record points at the
// env IP and the hostname is registered (no extra DNS record needed). If HTTP
// also fails we fall back to TXT validation (the interactive _dnsauth dance,
// usable from the CLI).
func issueManagedCertificate(ctx context.Context, client *armappcontainers.ManagedCertificatesClient, rg, envName, certName, hostname, location string, log func(string, ...any)) (*armappcontainers.ManagedCertificate, error) {
	resp, err := submitManagedCert(ctx, client, rg, envName, certName, hostname, location, armappcontainers.ManagedCertificateDomainControlValidationCNAME)
	if err == nil {
		return resp, nil
	}
	if !isInvalidValidationMethod(err) {
		return nil, fmt.Errorf("issuing managed certificate %s: %w", certName, err)
	}

	// Apex domain: CNAME validation is impossible. HTTP validation needs no extra
	// DNS record (the asuid TXT + A record already in place suffice), so it works
	// unattended in the CD task — unlike TXT, which requires an interactive
	// _dnsauth record.
	log("CNAME validation rejected (apex domain); using HTTP validation")
	resp, err = submitManagedCert(ctx, client, rg, envName, certName, hostname, location, armappcontainers.ManagedCertificateDomainControlValidationHTTP)
	if err == nil {
		return resp, nil
	}

	log("HTTP validation failed (%v); falling back to TXT validation", err)
	return submitManagedCertTXT(ctx, client, rg, envName, certName, hostname, location, log)
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
func submitManagedCertTXT(ctx context.Context, client *armappcontainers.ManagedCertificatesClient, rg, envName, certName, hostname, location string, log func(string, ...any)) (*armappcontainers.ManagedCertificate, error) {
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
		// Only swallow the "not yet there" case (404 while Azure is still
		// settling). Permission / auth / transient provider errors should
		// surface immediately instead of dragging out to the deadline.
		if getErr != nil {
			var respErr *azcore.ResponseError
			if !errors.As(getErr, &respErr) || respErr.StatusCode != 404 {
				return nil, fmt.Errorf("fetching managed certificate %s for validation token: %w", certName, getErr)
			}
		}
		if time.Now().After(tokenDeadline) {
			return nil, fmt.Errorf("timed out waiting for Azure to issue validationToken for %s", hostname)
		}
		if err := pkg.SleepWithContext(ctx, 5*time.Second); err != nil {
			return nil, err
		}
	}

	// Unlike the records from PreflightCert, this one can't be batched into the
	// up-front block: Azure only mints the token once the PUT is in flight.
	dnsauth := "_dnsauth." + hostname
	log("configure one more DNS record for managed cert validation:")
	log("  TXT    %s  ->  %s", dnsauth, token)
	log("waiting for %s and cert provisioning to finish (timeout ~5m)…", dnsauth)

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
// the app would be dropped by the PATCH. appLock guards this Get-modify-PATCH
// the same way it does in addHostnameDisabled — see appLocks doc comment.
func bindHostnameSniEnabled(ctx context.Context, client *armappcontainers.ContainerAppsClient, rg, appName, hostname, certID string) error {
	mu := lockForApp(rg, appName)
	mu.Lock()
	defer mu.Unlock()

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

// waitForTLS is silent on success: the caller's "waiting for TLS cert to come
// online…" line already framed the wait, and returning is the confirmation.
func waitForTLS(ctx context.Context, hostname string, resolver dns.Resolver) error {
	deadline := time.Now().Add(tlsWaitTimeout)
	for {
		if err := cert.CheckTLSCert(ctx, hostname, resolver); err == nil {
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
// a "--" run, which ARM rejects. A short hash of the original hostname is
// appended to the sanitized/truncated host so two long hostnames that share
// the same first ~21 sanitized characters don't collide on the same managed
// cert resource within an env (which would let the second issuance overwrite
// the first). sha256 is used as a non-cryptographic hash here — we only need
// determinism and a low collision rate over the inputs we'll see.
func managedCertName(envName, hostname string) string {
	env := sanitize(envName)
	if len(env) > 15 {
		env = strings.TrimRight(env[:15], "-")
	}
	host := sanitize(hostname)
	if len(host) > 21 {
		host = strings.TrimRight(host[:21], "-")
	}
	sum := sha256.Sum256([]byte(strings.ToLower(hostname)))
	suffix := hex.EncodeToString(sum[:4])
	return fmt.Sprintf("mc-%s-%s-%s", env, host, suffix)
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
