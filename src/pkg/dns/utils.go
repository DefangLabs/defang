package dns

import "strings"

func SafeLabel(fqn string) string {
	return strings.ReplaceAll(strings.ToLower(fqn), ".", "-")
}

func Normalize(domain string) string {
	return strings.ToLower(strings.TrimSuffix(domain, "."))
}

func IsSubdomain(subdomain, domain string) bool {
	subdomain = Normalize(subdomain)
	domain = Normalize(domain)

	if strings.HasSuffix(subdomain, "."+domain) {
		return true
	}
	return false
}
