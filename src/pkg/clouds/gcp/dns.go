package gcp

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"google.golang.org/api/dns/v1"
)

func (gcp Gcp) EnsureDNSZoneExists(ctx context.Context, name, domain, description string) (*dns.ManagedZone, error) {
	dnsSvc, err := dns.NewService(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to check DNS zone, failed to create DNS service: %w", err)
	}

	zoneSvc := dns.NewManagedZonesService(dnsSvc)
	zone, err := zoneSvc.Get(gcp.ProjectId, name).Do()
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
	}).Do(); err != nil {
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
	zone, err := zoneSvc.Get(gcp.ProjectId, name).Do()
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
	zone, err := zoneSvc.Get(gcp.ProjectId, name).Do()
	if err != nil {
		return fmt.Errorf("failed to get DNS zone service: %w", err)
	}

	rrsIterator := dnsSvc.ResourceRecordSets.List(gcp.ProjectId, name).Pages(ctx, func(page *dns.ResourceRecordSetsListResponse) error {
		for _, rrs := range page.Rrsets {
			if (rrs.Type == "NS" || rrs.Type == "SOA") && rrs.Name == zone.DnsName {
				continue // Skip NS and SOA records for the zone itself
			}
			_, err := dnsSvc.ResourceRecordSets.Delete(gcp.ProjectId, name, rrs.Name, rrs.Type).Do()
			if err != nil {
				return err
			}
		}
		return nil
	})

	if err := rrsIterator; err != nil {
		return fmt.Errorf("failed to delete DNS zone records: %w", err)
	}

	err = dnsSvc.ManagedZones.Delete(gcp.ProjectId, name).Do()
	if err != nil {
		return fmt.Errorf("failed to delete DNS zone: %w", err)
	}
	return nil
}

// 1. Name must be lowercase letters, numbers, and hyphens
// 2. Name may be at most 63 characters
// 3. Name must start with a lowercase letter
// 4. Name must end with a lowercase letter or a number
var safeZoneRE = regexp.MustCompile(`[^a-z0-9-]+`)

// Zone names have the same requirements as label values.
func SafeZoneName(input string) string {
	input = strings.ToLower(input)                  // Rule 1: lowercase
	safe := safeZoneRE.ReplaceAllString(input, "-") // Rule 1: only letters, numbers, and hyphen
	safe = strings.Trim(safe, "-")                  // Rule 3, 4: trim hyphens from start and end
	if len(safe) == 0 || safe[0] == '-' {
		safe = "zone" + safe
	}
	if safe[0] < 'a' || safe[0] > 'z' { // Rule 3: must start with a letter
		safe = "zone-" + safe
	}
	return hashTrim(safe, 63) // Rule 2: max length 63
}

func hashTrim(name string, maxLength int) string {
	if len(name) <= maxLength {
		return name
	}

	const hashLength = 6
	prefix := name[:maxLength-hashLength]
	suffix := name[maxLength-hashLength:]
	return prefix + hashn(suffix, hashLength)
}

func hashn(str string, length int) string {
	hash := sha256.New()
	hash.Write([]byte(str))
	hashInt := binary.LittleEndian.Uint64(hash.Sum(nil)[:8])
	hashBase36 := strconv.FormatUint(hashInt, 36) // base 36 string
	// truncate if the hash is too long
	if len(hashBase36) > length {
		return hashBase36[:length]
	}
	// if the hash is too short, pad with leading zeros
	return fmt.Sprintf("%0*s", length, hashBase36)
}
