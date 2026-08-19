package dns

import (
	"strings"

	"golang.org/x/net/publicsuffix"
)

func SafeLabel(fqn string) string {
	return strings.ReplaceAll(strings.ToLower(fqn), ".", "-")
}

func Normalize(domain string) string {
	return strings.ToLower(strings.TrimSuffix(domain, "."))
}

// IsApexDomain reports whether hostname is a registrable domain's apex/root
// (e.g. "example.com", "example.co.uk") rather than a subdomain of one (e.g.
// "www.example.com"). It's a static property of the name — no DNS lookup
// involved — so callers can use it to pick CNAME vs A record instructions
// before any DNS is configured. A hostname that isn't a valid registrable
// domain (e.g. a bare public suffix) is treated as non-apex: it can't be
// bound as a custom domain anyway.
func IsApexDomain(hostname string) bool {
	hostname = Normalize(hostname)
	etld1, err := publicsuffix.EffectiveTLDPlusOne(hostname)
	if err != nil {
		return false
	}
	return etld1 == hostname
}
