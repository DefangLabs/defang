// Package dns manages public Azure DNS zones used for Defang domain
// delegation. Creating a public zone yields a set of authoritative name
// servers; delegating a subdomain to Azure means pointing NS records in the
// parent zone at those servers. This mirrors the GCP provider's managed-zone
// approach (pkg/clouds/gcp/dns.go) — Azure has no equivalent of Route53's
// reusable delegation sets, so the zone itself is the unit of delegation.
package dns

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/dns/armdns"
	"github.com/DefangLabs/defang/src/pkg/clouds/azure"
	"github.com/DefangLabs/defang/src/pkg/term"
)

// DNS manages public Azure DNS zones within a single resource group.
type DNS struct {
	azure.Azure
	resourceGroupName string
}

// New builds a DNS client rooted in the given resource group. The Azure value
// is copied in full so an authenticated credential (Azure.Cred) propagates to
// the SDK calls, matching the keyvault/aca packages.
func New(resourceGroupName string, az azure.Azure) *DNS {
	return &DNS{Azure: az, resourceGroupName: resourceGroupName}
}

func (d *DNS) newZonesClient() (*armdns.ZonesClient, error) {
	cred, err := d.NewCreds()
	if err != nil {
		return nil, err
	}
	client, err := armdns.NewZonesClient(d.SubscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create DNS zones client: %w", err)
	}
	return client, nil
}

// EnsureZoneExists returns the authoritative name servers for the public DNS
// zone named domain, creating the zone if it does not already exist. Lookup
// runs first so an existing zone (and its records) is never disturbed —
// matching GCP's EnsureDNSZoneExists. DNS zones are global resources, so the
// resource group only determines ownership and billing, not latency.
func (d *DNS) EnsureZoneExists(ctx context.Context, domain string) ([]string, error) {
	client, err := d.newZonesClient()
	if err != nil {
		return nil, err
	}

	resp, err := client.Get(ctx, d.resourceGroupName, domain, nil)
	if err == nil {
		term.Debugf("DNS zone %q already exists", domain)
		return nameServers(resp.Zone), nil
	}
	var respErr *azcore.ResponseError
	if !errors.As(err, &respErr) || respErr.StatusCode != 404 {
		return nil, fmt.Errorf("looking up DNS zone %q: %w", domain, err)
	}

	term.Debugf("Creating public DNS zone %q in resource group %q", domain, d.resourceGroupName)
	created, err := client.CreateOrUpdate(ctx, d.resourceGroupName, domain, armdns.Zone{
		Location: to.Ptr("global"),
		Properties: &armdns.ZoneProperties{
			ZoneType: to.Ptr(armdns.ZoneTypePublic),
		},
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create DNS zone %q: %w", domain, err)
	}
	return nameServers(created.Zone), nil
}

// FindZone returns the ARM resource ID of the public DNS zone in the current
// subscription whose name is the longest DNS suffix of domain, or "" if no zone
// matches. It mirrors AWS's findZone (byoc/aws/byoc.go): when a service brings
// its own domain, the caller sets ServiceInfo.ZoneId so the CD/Pulumi program
// manages records directly in that zone instead of the ACME fallback.
//
// The lookup is subscription-wide and applies no ownership/tag filter (per the
// BYOD design): whichever existing zone is the closest parent of domain wins.
// The resource group is irrelevant here, so a DNS value built with New("", az)
// is fine. Azure has no cross-subscription equivalent of Route53's AssumeRole,
// so only the current subscription is searched.
func (d *DNS) FindZone(ctx context.Context, domain string) (string, error) {
	client, err := d.newZonesClient()
	if err != nil {
		return "", err
	}

	domain = strings.ToLower(strings.TrimSuffix(domain, "."))
	var bestID, bestName string
	pager := client.NewListPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return "", fmt.Errorf("listing DNS zones: %w", err)
		}
		for _, z := range page.Value {
			if z == nil || z.Name == nil || z.ID == nil {
				continue
			}
			name := strings.ToLower(*z.Name)
			// Match the domain itself or any parent zone (e.g. domain
			// "api.example.com" matches a zone named "example.com").
			if domain != name && !strings.HasSuffix(domain, "."+name) {
				continue
			}
			if len(name) > len(bestName) {
				bestName, bestID = name, *z.ID
			}
		}
	}
	if bestID == "" {
		term.Debugf("no DNS zone in subscription matches %q", domain)
		return "", nil
	}
	term.Debugf("DNS zone %q (%s) matches %q", bestName, bestID, domain)
	return bestID, nil
}

func nameServers(zone armdns.Zone) []string {
	// Consistent zero value: callers can rely on a non-nil empty slice when
	// there are no name servers, regardless of whether Properties was unset
	// or just empty.
	if zone.Properties == nil {
		return []string{}
	}
	servers := make([]string, 0, len(zone.Properties.NameServers))
	for _, ns := range zone.Properties.NameServers {
		if ns != nil {
			servers = append(servers, *ns)
		}
	}
	return servers
}
