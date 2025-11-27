package aws

import (
	"context"
	"errors"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/clouds/aws"
	"github.com/DefangLabs/defang/src/pkg/dns"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/aws/aws-sdk-go-v2/service/route53/types"
)

func prepareDomainDelegation(ctx context.Context, projectDomain, projectName, stack string, r53Client aws.Route53API) (nsServers []string, delegationSetId string, err error) {
	// There's four cases to consider:
	//  1. The subdomain zone does not exist: we create/get a delegation set and get its NS records and let CD/Pulumi create the hosted zone
	//  2. The subdomain zone exists:
	//    a. DEPRECATED: The zone was created by the older CLI: we need to get the NS records from the existing zone and pass to Fabric; no delegation set
	//    b. The zone was created by the new CD/Pulumi of the same project and stack: we get the create or get the delegation set using the zone
	//    c. The zone was created another way: we ignore it and create a new delegation set and let CD/Pulumi create the hosted zone
	//    d. The zone was created by a different stack: We need to create a new delegation set and let CD/Pulumi create the hosted zone

	var delegationSet *types.DelegationSet
	zones, err := aws.GetHostedZonesByName(ctx, projectDomain, r53Client)
	if err != nil {
		// The only acceptable error is that the zone was not found
		if !errors.Is(err, aws.ErrZoneNotFound) {
			return nil, "", err // TODO: we should not fail deployment if GetHostedZoneByName fails
		}
		term.Debugf("Zone %q not found, delegation set will be created", projectDomain)
	} else {
		// Case 2: Get the NS records for the existing subdomain zone
		delegationSet, err = getOrCreateDelegationSetByZones(ctx, zones, projectName, stack, r53Client)
		if err != nil {
			return nil, "", err
		}
	}

	if delegationSet == nil {
		// Case 1, 2c and 2d: zone of the projectDomain and stack doesn't exist: we'll create/get a delegation set and let CD/Pulumi create the hosted zone
		// Create a new delegation set. There's a race condition here, where two deployments could create two different delegation sets,
		// but this is acceptable because the next time the zone is deployed, we'll get the existing delegation set from the zone.
		delegationSet, err = createUsableDelegationSet(ctx, projectDomain, r53Client, dns.ResolverAt)
		if err != nil {
			return nil, "", err
		}
	}

	if len(delegationSet.NameServers) == 0 {
		return nil, "", errors.New("no NS records found for the delegation set") // should not happen
	}
	if delegationSet.Id != nil {
		term.Debug("Route53 delegation set ID:", *delegationSet.Id)
		delegationSetId = strings.TrimPrefix(*delegationSet.Id, "/delegationset/")
	}

	return delegationSet.NameServers, delegationSetId, nil
}

func createUsableDelegationSet(ctx context.Context, domain string, r53Client aws.Route53API, getResolverAt func(string) dns.Resolver) (*types.DelegationSet, error) {
	// Try up to 10 times to create a delegation set that is usable (i.e., none of its NS servers have conflicting records for the domain)
	// Chances of a conflict happened in a single try if aws have 2000 dns servers is about (1 - (1-4/2000)^4) ~ 0.8%
	// Chances of this happening in 10 consecutive tries if servers are randomly chosen is about 0.8%^5 ~ 3.2e-13, virtually impossible
	for range 5 {
		delegationSet, err := aws.CreateDelegationSet(ctx, nil, r53Client)
		if err != nil {
			return nil, err
		}
		// Verify that the delegation set is usable by checking if any of its NS servers contains conflicting records
		conflictFound := false
		for _, nsServer := range delegationSet.NameServers {
			resolver := getResolverAt(nsServer)
			records, err := resolver.LookupNS(ctx, domain)
			if err != nil {
				return nil, err
			}
			if len(records) > 0 {
				// Records found, meaning the NS server is conflicting
				term.Debugf("Delegation set NS server %q has conflicting records: %v", nsServer, records)
				conflictFound = true
				break
			}
		}
		if conflictFound {
			if err := aws.DeleteDelegationSet(ctx, delegationSet.Id, r53Client); err != nil {
				term.Debugf("Failed to delete conflicting delegation set %q: %v", *delegationSet.Id, err)
			}
		} else {
			return delegationSet, nil
		}
	}
	return nil, errors.New("failed to create a usable delegation set without conflicting NS records after multiple attempts")
}

func getOrCreateDelegationSetByZones(ctx context.Context, zones []*types.HostedZone, projectName, stack string, r53Client aws.Route53API) (*types.DelegationSet, error) {
	for _, zone := range zones {
		projectDomain := dns.Normalize(*zone.Name)

		tags, err := aws.GetHostedZoneTags(ctx, *zone.Id, r53Client)
		if err != nil {
			return nil, err // TODO: we should not fail deployment if GetHostedZoneTags fails
		}
		// Ignore zones that was created by an older CLI (2a), or another way (2c) or belong to a different project/stack (2d)
		if tags["defang:project"] != projectName || tags["defang:stack"] != stack {
			term.Debugf("ignored zone %q as it belongs to a different project/stack (%q/%q), skipping", projectDomain, tags["defang:project"], tags["defang:stack"])
			continue
		}

		// Case 2b: The zone belongs to the same project and stack: get the NS records from the existing zone
		var delegationSet *types.DelegationSet
		// Create or get the reusable delegation set for the existing subdomain zone
		delegationSet, err = aws.CreateDelegationSet(ctx, zone.Id, r53Client)
		if delegationSetAlreadyReusable := new(types.DelegationSetAlreadyReusable); errors.As(err, &delegationSetAlreadyReusable) {
			term.Debug("Route53 delegation set already created:", err)
			delegationSet, err = aws.GetDelegationSetByZone(ctx, zone.Id, r53Client)
		}
		if err != nil {
			return nil, err
		}

		return delegationSet, err
	}
	return nil, nil
}
