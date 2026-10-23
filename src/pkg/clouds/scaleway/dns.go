package scaleway

import (
	"context"
	"fmt"
)

// DNSZone represents a Scaleway DNS zone.
type DNSZone struct {
	Domain    string   `json:"domain"`
	Subdomain string   `json:"subdomain"`
	NS        []string `json:"ns"`
	ProjectID string   `json:"project_id"`
	Status    string   `json:"status"`
	UpdatedAt string   `json:"updated_at"`
}

type listDNSZonesResponse struct {
	DNSZones   []DNSZone `json:"dns_zones"`
	TotalCount int       `json:"total_count"`
}

const dnsBaseURL = apiBaseURL + "/domain/v2beta1"

// CreateDNSZone creates a new DNS zone for the given domain.
func (c *Client) CreateDNSZone(ctx context.Context, domain, subdomain, projectID string) (*DNSZone, error) {
	if projectID == "" {
		projectID = c.ProjectID
	}
	url := dnsBaseURL + "/dns-zones"
	body := map[string]string{
		"domain":     domain,
		"subdomain":  subdomain,
		"project_id": projectID,
	}
	var zone DNSZone
	if err := c.doRequestJSON(ctx, "POST", url, body, &zone); err != nil {
		return nil, AnnotateScalewayError(err, fmt.Sprintf("creating DNS zone for %q", domain))
	}
	return &zone, nil
}

// GetDNSZone retrieves a DNS zone by domain name.
func (c *Client) GetDNSZone(ctx context.Context, domain string) (*DNSZone, error) {
	url := fmt.Sprintf("%s/dns-zones?domain=%s", dnsBaseURL, domain)
	var resp listDNSZonesResponse
	if err := c.doRequestJSON(ctx, "GET", url, nil, &resp); err != nil {
		return nil, AnnotateScalewayError(err, fmt.Sprintf("getting DNS zone for %q", domain))
	}
	if len(resp.DNSZones) == 0 {
		return nil, &APIError{StatusCode: 404, Message: fmt.Sprintf("DNS zone %q not found", domain)}
	}
	return &resp.DNSZones[0], nil
}

// DeleteDNSZone deletes a DNS zone by its full domain identifier.
func (c *Client) DeleteDNSZone(ctx context.Context, dnsZoneID string) error {
	url := fmt.Sprintf("%s/dns-zones/%s", dnsBaseURL, dnsZoneID)
	if err := c.doRequestJSON(ctx, "DELETE", url, nil, nil); err != nil {
		return AnnotateScalewayError(err, fmt.Sprintf("deleting DNS zone %q", dnsZoneID))
	}
	return nil
}
