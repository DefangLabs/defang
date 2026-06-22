package cli

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"
	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cert"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/dns"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"golang.org/x/sync/errgroup"
)

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type DNSResult struct {
	IPs    []net.IPAddr
	Expiry time.Time
}

var (
	// dnsCacheMu guards dnsCache. Workers now run in parallel (see
	// runACMEJobs), so newCertHTTPClient's DialContext closure — captured by
	// every worker's HTTP client — can race on this map without it.
	dnsCacheMu       sync.RWMutex
	dnsCache         = make(map[string]DNSResult)
	dnsCacheDuration = 1 * time.Minute

	httpRetryDelayBase = 5 * time.Second
)

// maxCertWorkers caps concurrent per-domain cert workers. Per-domain work is
// mostly idle wait (DNS propagation, ACME issuance), so the cap exists to keep
// the log stream readable rather than for throughput.
const maxCertWorkers = 8

func newCertHTTPClient(r dns.Resolver) HTTPClient {
	return &http.Client{
		// Based on the default transport: https://pkg.go.dev/net/http#RoundTripper
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				host, port, err := net.SplitHostPort(addr)
				if err != nil {
					return nil, err
				}
				dnsCacheMu.RLock()
				cached, ok := dnsCache[host]
				dnsCacheMu.RUnlock()
				var ips []net.IPAddr
				if ok && cached.Expiry.After(time.Now()) {
					ips = cached.IPs
				} else {
					ips, err = r.LookupIPAddr(ctx, host)
					if err != nil {
						return nil, err
					}
					// Keep 1min of dns cache to avoid spamming root dns servers.
					// A racing concurrent lookup may overwrite this entry with
					// fresh data — that's fine; both writes are valid.
					expiry := time.Now().Add(dnsCacheDuration)
					dnsCacheMu.Lock()
					dnsCache[host] = DNSResult{ips, expiry}
					dnsCacheMu.Unlock()
				}

				dialer := &net.Dialer{}
				rootAddr := net.JoinHostPort(ips[pkg.RandomIndex(len(ips))].String(), port)
				return dialer.DialContext(ctx, network, rootAddr)
			},
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			term.Debugf("Redirecting from %v to %v", via[len(via)-1].URL, req.URL)
			return nil
		},
	}
}

// CertIssuer is implemented by providers that issue and bind TLS certificates
// directly against their cloud's API rather than going through the
// CNAME→fabric→ACME redirect dance used on AWS BYOD / Playground. Azure
// implements this so `defang cert generate` can drive the Container Apps
// hostname-add + managed-cert + SniEnabled-bind sequence end-to-end.
type CertIssuer interface {
	IssueCert(ctx context.Context, projectName, serviceName, hostname string, resolverAt func(string) dns.Resolver) error
}

// domainJob is one (service, domain, targets) tuple processed by a worker.
// targets are pre-normalized at collection time so workers can read them
// concurrently without each performing their own in-place normalization.
type domainJob struct {
	serviceName string
	domain      string
	targets     []string
}

func GenerateLetsEncryptCert(ctx context.Context, project *compose.Project, fab client.FabricClient, provider client.Provider) error {
	term.Debugf("Generating TLS cert for project %q", project.Name)

	services, err := provider.GetServices(ctx, &defangv1.GetServicesRequest{Project: project.Name})
	if err != nil {
		return err
	}
	if len(services.Services) == 0 {
		return fmt.Errorf("no services found for project %q; deployment may not be finished yet", project.Name)
	}

	issuer, _ := provider.(CertIssuer)
	jobs := collectDomainJobs(project, services.Services)
	if len(jobs) == 0 {
		term.Infof("No `domainname` found in compose file; no HTTPS cert generation needed")
		return nil
	}

	if issuer != nil {
		return runIssuerJobs(ctx, project.Name, jobs, fab, issuer)
	}
	return runACMEJobs(ctx, jobs, fab)
}

// collectDomainJobs flattens services into per-domain work items. Validation
// warnings (domain mismatch, missing domainname) are emitted here so they
// appear once, up-front, before any parallel worker output.
func collectDomainJobs(project *compose.Project, services []*defangv1.ServiceInfo) []domainJob {
	var jobs []domainJob
	for _, si := range services {
		if !si.UseAcmeCert {
			continue
		}
		svc, ok := project.Services[si.Service.Name]
		if !ok {
			continue
		}
		if svc.DomainName != si.Domainname {
			term.Warnf("service %q: domainname %q in compose file does not match deployed value %q, will use the deployed value", svc.Name, svc.DomainName, si.Domainname)
		}
		if si.Domainname == "" {
			term.Warnf("service %q: `domainname` is deployed without a domainname, skipping cert generation", svc.Name)
			continue
		}
		// Pre-normalize once so the worker's wait loop can read the slice
		// without racing other workers on shared backing storage.
		rawTargets := getDomainTargets(si, svc)
		targets := make([]string, len(rawTargets))
		for i, t := range rawTargets {
			targets[i] = dns.Normalize(t)
		}

		domains := []string{si.Domainname}
		if defaultNetwork := svc.Networks["default"]; defaultNetwork != nil {
			domains = append(domains, defaultNetwork.Aliases...)
		}
		for _, d := range domains {
			jobs = append(jobs, domainJob{serviceName: svc.Name, domain: d, targets: targets})
		}
	}
	return jobs
}

func getDomainTargets(serviceInfo *defangv1.ServiceInfo, service compose.ServiceConfig) []string {
	// Only use the ALB for aws cert gen to avoid defang domain in the middle
	if serviceInfo.LbDnsName != "" {
		return []string{serviceInfo.LbDnsName}
	} else {
		targets := []string{serviceInfo.PublicFqdn}
		for i, endpoint := range serviceInfo.Endpoints {
			if service.Ports[i].Mode == compose.Mode_INGRESS {
				targets = append(targets, endpoint)
			}
		}
		return targets
	}
}

// runIssuerJobs runs the provider-driven cert issuance in parallel. Per-domain
// state transitions are emitted via a per-domain prefixed logger so the log
// stream remains readable across concurrent workers.
func runIssuerJobs(ctx context.Context, projectName string, jobs []domainJob, fab client.FabricClient, issuer CertIssuer) error {
	pad := maxDomainLen(jobs)
	eg, gctx := errgroup.WithContext(ctx)
	eg.SetLimit(maxCertWorkers)
	var (
		errMu sync.Mutex
		errs  []error
	)
	for _, j := range jobs {
		job := j
		log := newDomainLogger(job.domain, pad)
		eg.Go(func() error {
			start := time.Now()
			log("issuing cert…")
			if err := issuer.IssueCert(gctx, projectName, job.serviceName, job.domain, dns.NewFabricResolverAt(fab)); err != nil {
				log("failed: %v", err)
				errMu.Lock()
				errs = append(errs, fmt.Errorf("%v: %w", job.domain, err))
				errMu.Unlock()
				return nil // don't abort sibling workers
			}
			log("cert issued ✓ (%s)", time.Since(start).Round(time.Second))
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		return err
	}
	if len(errs) > 0 {
		return fmt.Errorf("certificate issuance failed for one or more domains; verify DNS records and retry `defang cert generate`: %w", errors.Join(errs...))
	}
	return nil
}

// runACMEJobs runs the fabric/ACME redirect-dance flow in two phases:
// Phase 1 — sequential pre-flight VerifyDNSSetup across all domains, then a
// single grouped CNAME/ALIAS instruction block for the ones not yet ready,
// so the user can configure every record in one DNS-console sitting.
// Phase 2 — parallel per-domain workers (DNS wait, cert trigger, TLS wait)
// emitting one prefixed status line per state transition.
func runACMEJobs(ctx context.Context, jobs []domainJob, fab client.FabricClient) error {
	pad := maxDomainLen(jobs)

	// Phase 1: pre-flight.
	verified := preflightDNS(ctx, jobs, fab)
	var needsSetup []domainJob
	for _, j := range jobs {
		if verified[j.domain] {
			newDomainLogger(j.domain, pad)("DNS already configured")
		} else {
			needsSetup = append(needsSetup, j)
		}
	}
	if len(needsSetup) > 0 {
		printGroupedCNAMEs(needsSetup, pad)
		term.Infof("Awaiting DNS record setup and propagation for %d domain(s)…", len(needsSetup))
	}

	// Phase 2: parallel workers.
	eg, gctx := errgroup.WithContext(ctx)
	eg.SetLimit(maxCertWorkers)
	var (
		errMu sync.Mutex
		errs  []error
	)
	for _, j := range jobs {
		job := j
		alreadyVerified := verified[job.domain]
		log := newDomainLogger(job.domain, pad)
		eg.Go(func() error {
			if err := runACMEForDomain(gctx, job, fab, log, alreadyVerified); err != nil {
				log("failed: %v", err)
				errMu.Lock()
				errs = append(errs, fmt.Errorf("%v: %w", job.domain, err))
				errMu.Unlock()
			}
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		return err
	}
	if len(errs) > 0 {
		return fmt.Errorf("certificate issuance failed for one or more domains; verify DNS records and retry `defang cert generate`: %w", errors.Join(errs...))
	}
	return nil
}

// preflightDNS asks the server to verify each domain's CNAME/ALIAS setup once,
// returning the set of domains the server is happy with. Negative results
// (CodeFailedPrecondition) and RPC failures are both treated as "not yet
// verified" — the per-domain worker will keep retrying and supplement with
// local DNS checks.
func preflightDNS(ctx context.Context, jobs []domainJob, fab client.FabricClient) map[string]bool {
	verified := make(map[string]bool, len(jobs))
	var mu sync.Mutex
	eg, gctx := errgroup.WithContext(ctx)
	eg.SetLimit(maxCertWorkers)
	for _, j := range jobs {
		job := j
		eg.Go(func() error {
			if err := fab.VerifyDNSSetup(gctx, &defangv1.VerifyDNSSetupRequest{Domain: job.domain, Targets: job.targets}); err == nil {
				mu.Lock()
				verified[job.domain] = true
				mu.Unlock()
			} else {
				term.Debugf("Pre-flight DNS verification for %v not yet ready: %v", job.domain, err)
			}
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		return nil // ignore errors, this should never happen
	}
	return verified
}

// printGroupedCNAMEs prints every pending domain's record on its own line, in
// a single block. Padding aligns the arrow column so multi-domain projects
// read like a small table.
func printGroupedCNAMEs(jobs []domainJob, pad int) {
	term.Infof("Configure the following DNS record(s) (CNAME or ALIAS; any listed target per row works):")
	for _, j := range jobs {
		term.Printf("  %-*s  ->  %s\n", pad, j.domain, strings.Join(j.targets, " or "))
	}
}

// runACMEForDomain runs the per-domain ACME flow: wait for DNS, probe for an
// existing cert, trigger generation, wait for the cert to come online. All
// inline progress goes through log so concurrent workers don't fight for the
// TTY. If alreadyVerified, the DNS wait step is skipped — Phase 1 just saw it.
func runACMEForDomain(ctx context.Context, job domainJob, fab client.FabricClient, log func(string, ...any), alreadyVerified bool) error {
	r := dns.FabricResolver{Client: fab}
	start := time.Now()

	if !alreadyVerified {
		if err := waitForDNSVerified(ctx, job.domain, job.targets, fab, log); err != nil {
			return fmt.Errorf("waiting for DNS verification: %w", err)
		}
		log("DNS verified")
	}

	if err := cert.CheckTLSCert(ctx, job.domain, r); err == nil {
		log("TLS cert already ready (%s)", time.Since(start).Round(time.Second))
		return nil
	}
	if err := pkg.SleepWithContext(ctx, 5*time.Second); err != nil {
		return fmt.Errorf("waiting for DNS propagation: %w", err)
	}

	log("triggering cert generation…")
	if err := triggerCertGen(ctx, job.domain, r); err != nil {
		return fmt.Errorf("triggering cert generation: %w", err)
	}

	log("waiting for TLS cert to come online…")
	if err := waitForTLSReady(ctx, job.domain, r); err != nil {
		return fmt.Errorf("waiting for TLS to come online: %w", err)
	}
	log("TLS online ✓ (%s)", time.Since(start).Round(time.Second))
	return nil
}

// waitForDNSVerified blocks until the server-side or local DNS check confirms
// the record is in place, or ctx expires. Quiet by design — the caller logs
// state transitions; this function only logs the propagation caveat because
// it materially changes user expectations (the server is happy but the local
// DNS may still be stale).
func waitForDNSVerified(ctx context.Context, domain string, targets []string, fab client.FabricClient, log func(string, ...any)) error {
	serverSideVerified := false
	serverVerifyRpcFailure := 0
	propagationNoted := false

	verify := func() bool {
		if !serverSideVerified {
			err := fab.VerifyDNSSetup(ctx, &defangv1.VerifyDNSSetupRequest{Domain: domain, Targets: targets})
			switch {
			case err == nil:
				serverSideVerified = true
			case isNegativeVerify(err):
				// Server says "not yet" — it's authoritative, so keep polling it.
				term.Debugf("Server side DNS verification for %v negative: %v", domain, err)
			default:
				// Transient RPC failure: keep re-probing the server on later
				// ticks (a recovered fabric is picked up) but, once it has been
				// unreachable enough times, fall back to local DNS below so a
				// server outage doesn't block issuance indefinitely.
				serverVerifyRpcFailure++
				term.Debugf("Server side DNS verification request for %v failed (%d): %v", domain, serverVerifyRpcFailure, err)
			}
		}
		if serverSideVerified || serverVerifyRpcFailure >= 3 {
			locallyVerified := dns.CheckDomainDNSReady(ctx, domain, targets, dns.NewFabricResolverAt(fab))
			if serverSideVerified && !locallyVerified {
				if !propagationNoted {
					log("DNS verified server-side; local caches still propagating (may take a few minutes)")
					propagationNoted = true
				}
				return true // server-side trust wins; cert work can proceed
			}
			return locallyVerified
		}
		return false
	}

	if verify() {
		return nil
	}
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if verify() {
				return nil
			}
		}
	}
}

func isNegativeVerify(err error) bool {
	cerr := new(connect.Error)
	return errors.As(err, &cerr) && cerr.Code() == connect.CodeFailedPrecondition
}

// triggerCertGen sends the HTTP GET that nudges the fabric into running the
// ACME challenge for this domain. No spinner: callers running concurrently
// would clobber the TTY; the worker's prefixed log line is the progress signal.
func triggerCertGen(ctx context.Context, domain string, r dns.Resolver) error {
	if err := getWithRetries(ctx, fmt.Sprintf("http://%v", domain), 5, newCertHTTPClient(r)); err != nil {
		term.Debugf("Error triggering cert generation for %v: %v", domain, err)
		return err
	}
	return nil
}

// waitForTLSReady polls https://domain on a 3s ticker until the TLS handshake
// succeeds or the 10-minute budget is exhausted. The outer ctx still bounds
// the total wait.
func waitForTLSReady(ctx context.Context, domain string, r dns.Resolver) error {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	deadline, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	for {
		select {
		case <-deadline.Done():
			return deadline.Err()
		case <-ticker.C:
			if err := cert.CheckTLSCert(deadline, domain, r); err == nil {
				return nil
			} else {
				term.Debugf("Error checking TLS cert for %v: %v", domain, err)
			}
		}
	}
}

// maxDomainLen returns the longest domain across jobs, used to right-pad
// per-domain log prefixes so columns line up. Returns 0 when there are no
// jobs (callers handle that earlier, but be defensive).
func maxDomainLen(jobs []domainJob) int {
	n := 0
	for _, j := range jobs {
		if len(j.domain) > n {
			n = len(j.domain)
		}
	}
	return n
}

// newDomainLogger returns a closure that prefixes every line with [domain]
// padded to a fixed width so concurrent workers' output reads as a clean
// stream of per-domain events. term.Infof writes one Fprintf per call, which
// is goroutine-safe for our single-line events.
func newDomainLogger(domain string, pad int) func(format string, args ...any) {
	prefix := "[" + domain + "]" + strings.Repeat(" ", pad-len(domain)+1)
	return func(format string, args ...any) {
		term.Infof(prefix+format, args...)
	}
}

func getWithRetries(ctx context.Context, url string, tries int, c HTTPClient) error {
	var errs []error
	for i := range make([]struct{}, tries) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err // No point retrying if we can't even create the request
		}
		resp, err := c.Do(req)
		if err == nil {
			defer resp.Body.Close()
			_, err = io.ReadAll(resp.Body) // Read the body to ensure the request is not swallowed by alb
			if resp.StatusCode == http.StatusOK {
				return nil
			}
			if resp != nil && resp.Request != nil && resp.Request.URL.Scheme == "https" {
				term.Debugf("cert gen request success, received redirect to %v", resp.Request.URL)
				return nil // redirect to https indicate a successful cert generation
			}
			if err == nil {
				err = fmt.Errorf("HTTP: %v", resp.StatusCode)
			}
		} else if cve := new(tls.CertificateVerificationError); errors.As(err, &cve) {
			term.Debugf("cert gen request success, received tls error: %v", cve)
			return nil // tls error indicate a successful cert gen trigger, as it has to be redirected to https
		}

		term.Debugf("Error fetching %v: %v, tries left %v", url, err, tries-i-1)
		errs = append(errs, err)

		delay := httpRetryDelayBase << i // Simple exponential backoff
		if err := pkg.SleepWithContext(ctx, delay); err != nil {
			return err
		}
	}
	return errors.Join(errs...)
}
