package gcp

import (
	"context"
	"fmt"

	"google.golang.org/api/dns/v1"
)

func (gcp Gcp) EnsureDNSZoneExists(ctx context.Context, name, domain, description string) (*dns.ManagedZone, error) {
	dnsSvc, err := dns.NewService(ctx)
	if err != nil {
		return nil, fmt.Errorf("dns.NewService: %w", err)
	}

	zoneSvc := dns.NewManagedZonesService(dnsSvc)
	zone, err := zoneSvc.Get(gcp.ProjectId, name).Do()
	if err == nil {
		return zone, nil
	}

	if !IsNotFound(err) {
		return nil, fmt.Errorf("zoneSvc.Get: %w", err)
	}

	if domain[len(domain)-1] != '.' {
		domain += "."
	}
	if zone, err := zoneSvc.Create(gcp.ProjectId, &dns.ManagedZone{
		Name:        name,
		DnsName:     domain,
		Description: description,
	}).Do(); err != nil {
		return nil, fmt.Errorf("zoneSvc.Create: %w", err)
	} else {
		return zone, nil
	}
}

func (gcp Gcp) GetDNSZone(ctx context.Context, name string) (*dns.ManagedZone, error) {
	dnsSvc, err := dns.NewService(ctx)
	if err != nil {
		return nil, fmt.Errorf("dns.NewService: %w", err)
	}

	zoneSvc := dns.NewManagedZonesService(dnsSvc)
	zone, err := zoneSvc.Get(gcp.ProjectId, name).Do()
	if err != nil {
		return nil, fmt.Errorf("zoneSvc.Get: %w", err)
	}

	return zone, nil
}