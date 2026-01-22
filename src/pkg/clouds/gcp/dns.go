package gcp

import (
	"context"
	"fmt"

	"google.golang.org/api/dns/v1"
)

func (gcp Gcp) getDNSZone(ctx context.Context, name string) (*dns.ManagedZone, *dns.Service, error) {
	dnsSvc, err := dns.NewService(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create DNS service: %w", err)
	}

	zoneSvc := dns.NewManagedZonesService(dnsSvc)
	// This requires dns.googleapis.com service to be enabled in the GCP project
	zone, err := zoneSvc.Get(gcp.ProjectId, name).Context(ctx).Do()
	if err != nil {
		if IsAccessNotEnabled(err) {
			if err := gcp.EnsureAPIsEnabled(ctx, "dns.googleapis.com"); err != nil {
				return nil, nil, err
			}
			// Retry after enabling the API
			zone, err = zoneSvc.Get(gcp.ProjectId, name).Context(ctx).Do()
		}
	}
	return zone, dnsSvc, err
}

func (gcp Gcp) EnsureDNSZoneExists(ctx context.Context, name, domain, description string) (*dns.ManagedZone, error) {
	zone, dnsSvc, err := gcp.getDNSZone(ctx, name)
	if err != nil {
		if !IsNotFound(err) {
			return nil, fmt.Errorf("failed to get DNS managed zone: %w", err)
		}
	} else {
		return zone, nil
	}

	if domain[len(domain)-1] != '.' {
		domain += "."
	}
	zoneSvc := dns.NewManagedZonesService(dnsSvc)
	if zone, err := zoneSvc.Create(gcp.ProjectId, &dns.ManagedZone{
		Name:        name,
		DnsName:     domain,
		Description: description,
	}).Context(ctx).Do(); err != nil {
		return nil, fmt.Errorf("failed to create managed zone: %w", err)
	} else {
		return zone, nil
	}
}

func (gcp Gcp) GetDNSZone(ctx context.Context, name string) (*dns.ManagedZone, error) {
	zone, _, err := gcp.getDNSZone(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get DNS zone: %w", err)
	}
	return zone, nil
}

func (gcp Gcp) DeleteDNSZone(ctx context.Context, name string) error {
	zone, dnsSvc, err := gcp.getDNSZone(ctx, name)
	if err != nil {
		return fmt.Errorf("failed to get DNS zone: %w", err)
	}

	rrsIterator := dnsSvc.ResourceRecordSets.List(gcp.ProjectId, name).Pages(ctx, func(page *dns.ResourceRecordSetsListResponse) error {
		for _, rrs := range page.Rrsets {
			if (rrs.Type == "NS" || rrs.Type == "SOA") && rrs.Name == zone.DnsName {
				continue // Skip NS and SOA records for the zone itself
			}
			_, err := dnsSvc.ResourceRecordSets.Delete(gcp.ProjectId, name, rrs.Name, rrs.Type).Context(ctx).Do()
			if err != nil {
				return err
			}
		}
		return nil
	})

	if err := rrsIterator; err != nil {
		return fmt.Errorf("failed to delete DNS zone records: %w", err)
	}

	err = dnsSvc.ManagedZones.Delete(gcp.ProjectId, name).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("failed to delete DNS zone: %w", err)
	}
	return nil
}
