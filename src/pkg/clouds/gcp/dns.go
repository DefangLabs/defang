package gcp

import (
	"context"
	"fmt"

	"google.golang.org/api/dns/v1"
)

func (gcp Gcp) EnsureDNSZoneExists(ctx context.Context, name, domain, description string) (*dns.ManagedZone, error) {
	dnsSvc, err := dns.NewService(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to check DNS zone, failed to create DNS service: %w", err)
	}

	zoneSvc := dns.NewManagedZonesService(dnsSvc)
	zone, err := zoneSvc.Get(gcp.ProjectId, name).Context(ctx).Do()
	if err == nil {
		return zone, nil
	}

	if !IsNotFound(err) {
		return nil, fmt.Errorf("failed to get DNS managed zone service: %w", err)
	}

	if domain[len(domain)-1] != '.' {
		domain += "."
	}
	if zone, err := zoneSvc.Create(gcp.ProjectId, &dns.ManagedZone{
		Name:        name,
		DnsName:     domain,
		Description: description,
	}).Context(ctx).Do(); err != nil {
		return nil, fmt.Errorf("failed to create zone service: %w", err)
	} else {
		return zone, nil
	}
}

func (gcp Gcp) GetDNSZone(ctx context.Context, name string) (*dns.ManagedZone, error) {
	dnsSvc, err := dns.NewService(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to get DNS zone, failed to create DNS service: %w", err)
	}

	zoneSvc := dns.NewManagedZonesService(dnsSvc)
	zone, err := zoneSvc.Get(gcp.ProjectId, name).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to get DNS zone service: %w", err)
	}

	return zone, nil
}

func (gcp Gcp) DeleteDNSZone(ctx context.Context, name string) error {
	dnsSvc, err := dns.NewService(ctx)
	if err != nil {
		return fmt.Errorf("failed to create DNS service: %w", err)
	}

	zoneSvc := dns.NewManagedZonesService(dnsSvc)
	zone, err := zoneSvc.Get(gcp.ProjectId, name).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("failed to get DNS zone service: %w", err)
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
