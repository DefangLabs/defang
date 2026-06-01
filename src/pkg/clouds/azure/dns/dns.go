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

func nameServers(zone armdns.Zone) []string {
	if zone.Properties == nil {
		return nil
	}
	servers := make([]string, 0, len(zone.Properties.NameServers))
	for _, ns := range zone.Properties.NameServers {
		if ns != nil {
			servers = append(servers, *ns)
		}
	}
	return servers
}
