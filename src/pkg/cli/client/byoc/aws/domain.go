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

func prepareDomainDelegation(ctx context.Context, projectDomain, projectName, stackName string, r53Client aws.Route53API, resolverAt func(string) dns.Resolver) (nsServers []string, delegationSetId string, err error) {
	// There's four cases to consider:
	//  1. The subdomain zone does not exist: we create/get a delegation set and get its NS records and let CD/Pulumi create the hosted zone
	//  2. The subdomain zone exists:
	//    a. DEPRECATED: The zone was created by the older CLI: we consider the existing zone not usable, create a new delegation set and let CD/Pulumi create the hosted zone
	//    b. The zone was created by the new CD/Pulumi of the same project and stack: we create or get the delegation set using the zone
	//    c. The zone was created another way: we ignore it and create a new delegation set and let CD/Pulumi create the hosted zone
	//    d. The zone was created by a different stack: We need to create a new delegation set and let CD/Pulumi create the hosted zone

	var delegationSet *types.DelegationSet
	zones, err := aws.GetHostedZonesByName(ctx, projectDomain, r53Client)
	if err != nil {
		// The only acceptable error is that the zone was not found
		if !errors.Is(err, aws.ErrZoneNotFound) {
			return nil, "", err // TODO: we should not fail deployment if GetHostedZonesByName fails
		}
		term.Debugf("Zone %q not found, delegation set will be created", projectDomain)
	} else {
		// Case 2: Get the NS records for the existing subdomain zone
		delegationSet, err = getOrCreateDelegationSetByZones(ctx, zones, projectName, stackName, r53Client)
		if err != nil {
			return nil, "", err
		}
	}

	if delegationSet == nil {
		// Case 1, 2c and 2d: zone of the projectDomain and stack doesn't exist: we'll create/get a delegation set and let CD/Pulumi create the hosted zone
		// Create a new delegation set. There's a race condition here, where two deployments could create two different delegation sets,
		// but this is acceptable because the next time the zone is deployed, we'll get the existing delegation set from the zone.
		delegationSet, err = findUsableDelegationSet(ctx, projectDomain, r53Client, resolverAt)
		if err != nil {
			term.Warnf("Failed to find existing usable delegation set: %v, creating a new one", err)
		}
		if delegationSet != nil {
			term.Debug("Reusing existing usable Route53 delegation set:", *delegationSet.Id)
		} else {
			delegationSet, err = createUsableDelegationSet(ctx, projectDomain, r53Client, resolverAt)
			if err != nil {
				return nil, "", err
			}
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

func findUsableDelegationSet(ctx context.Context, domain string, r53Client aws.Route53API, resolverAt func(string) dns.Resolver) (*types.DelegationSet, error) {
	// List existing delegation sets and check if any are usable (i.e., none of its NS servers have conflicting records for the domain)
	delegationSets, err := aws.ListReusableDelegationSets(ctx, r53Client)
	if err != nil {
		return nil, err
	}
	for _, delegationSet := range delegationSets {
		// Verify that the delegation set is usable by checking that none of its NS servers contain records for this domain
		conflictFound, err := nameServersHasConflict(ctx, delegationSet.NameServers, []string{domain, "defang.app"}, resolverAt) // defang.app is also considered a conflict
		if err != nil {
			return nil, err
		}
		if conflictFound {
			continue
		}
		hostedZones, err := aws.ListHostedZonesByDelegationSet(ctx, delegationSet.Id, r53Client)
		if err != nil {
			return nil, err
		}
		if len(hostedZones) >= 100 {
			// A delegation set can only be associated with up to 100 hosted zones by default
			// (https://docs.aws.amazon.com/Route53/latest/DeveloperGuide/DNSLimitations.html#limits-api-entities-hosted-zones)
			term.Debugf("Delegation set %q has reached the maximum number of hosted zones (100), skipping", *delegationSet.Id)
			continue
		}
		return &delegationSet, nil
	}
	return nil, nil
}

func createUsableDelegationSet(ctx context.Context, domain string, r53Client aws.Route53API, resolverAt func(string) dns.Resolver) (*types.DelegationSet, error) {
	// route53 assigns a random selection of name servers when creating a
	// delegation set. If we get a delegation set which contains a name server
	// which already has an NS record for this hosted zone, its a conflict since
	// the server cannot serve more than one hosted zone for the same domain.
	// Try up to 5 times to create a delegation set that is usable (i.e., none
	// of its NS servers have conflicting records for the domain)
	// Chances of a conflict happening in a single try if aws have 2000 dns servers is about (1 - (1-4/2000)^4) ~ 0.8%
	// Chances of this happening in 5 consecutive tries if servers are randomly chosen is about 0.8%^5 ~ 3.2e-13, virtually impossible
	for range 5 {
		delegationSet, err := aws.CreateDelegationSet(ctx, nil, r53Client)
		if err != nil {
			return nil, err
		}
		// Verify that the delegation set is usable by checking that none of its NS servers contain records for this domain
		conflictFound, err := nameServersHasConflict(ctx, delegationSet.NameServers, []string{domain, "defang.app"}, resolverAt) // defang.app is also considered a conflict
		if err != nil {
			return nil, err
		}
		if conflictFound {
			if err := aws.DeleteDelegationSet(ctx, delegationSet.Id, r53Client); err != nil {
				// up to 100 delegation sets can be created per account, failure is non-fatal
				// there is no direct actionable remedy for the user too.
				// TODO: find and reuse empty delegation sets to avoid hitting the limit
				term.Debugf("Failed to delete conflicting delegation set %q: %v", *delegationSet.Id, err)
			}
		} else {
			return delegationSet, nil
		}
	}
	return nil, errors.New("failed to create a usable delegation set without conflicting NS records after multiple attempts")
}

func nameServersHasConflict(ctx context.Context, nameServers []string, domains []string, resolverAt func(string) dns.Resolver) (bool, error) {
	for _, nsServer := range nameServers {
		resolver := resolverAt(nsServer)

		for _, domain := range domains {
			if records, err := resolver.LookupNS(ctx, domain); err != nil {
				return false, err
			} else if len(records) > 0 {
				// Records found, meaning the NS server is conflicting
				term.Debugf("Name server %q has conflicting records for domain %q: %v", nsServer, domain, records)
				return true, nil
			}
		}
	}
	return false, nil
}

func getOrCreateDelegationSetByZones(ctx context.Context, zones []*types.HostedZone, projectName, stackName string, r53Client aws.Route53API) (*types.DelegationSet, error) {
	for _, zone := range zones {
		projectDomain := dns.Normalize(*zone.Name)

		tags, err := aws.GetHostedZoneTags(ctx, *zone.Id, r53Client)
		if err != nil {
			return nil, err // TODO: we should not fail deployment if GetHostedZoneTags fails
		}
		// Ignore zones that were created by an older CLI (2a), or another way (2c) or belong to a different project/stack (2d)
		if tags["defang:project"] != projectName || tags["defang:stack"] != stackName {
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

		return delegationSet, nil
	}
	return nil, nil
}
