package aws

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/clouds/aws"
	"github.com/DefangLabs/defang/src/pkg/dns"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/track"
	"github.com/aws/aws-sdk-go-v2/service/route53/types"
)

func prepareDomainDelegation(ctx context.Context, projectDomain string, r53Client aws.Route53API) (nsServers []string, delegationSetId string, err error) {
	// There's four cases to consider:
	//  1. The subdomain zone does not exist: we create/get a delegation set and get its NS records and let CD/Pulumi create the hosted zone
	//  2. The subdomain zone exists:
	//    a. The zone was created by the older CLI: we need to get the NS records from the existing zone and pass to Fabric; no delegation set
	//    b. The zone was created by the new CD/Pulumi: we get the NS records from the delegation set and let CD/Pulumi create/update the hosted zone
	//    c. The zone was created another way: get the NS records from the existing zone and pass to Fabric; no delegation set

	var delegationSet *types.DelegationSet
	zone, err := aws.GetHostedZoneByName(ctx, projectDomain, r53Client)
	if err != nil {
		// The only acceptable error is that the zone was not found
		if !errors.Is(err, aws.ErrZoneNotFound) {
			return nil, "", err // TODO: we should not fail deployment if GetHostedZoneByName fails
		}
		term.Debugf("Zone %q not found, delegation set will be created", projectDomain)

		// Case 1: The zone doesn't exist: we'll create/get a delegation set and let CD/Pulumi create the hosted zone
		delegationSet, err = getOrCreateDelegationSet(ctx, r53Client)
		if err != nil {
			return nil, "", err
		}
	} else {
		// Case 2: Get the NS records for the existing subdomain zone
		delegationSet, err = getOrCreateDelegationSetByZone(ctx, zone, r53Client)
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

func getOrCreateDelegationSet(ctx context.Context, r53Client aws.Route53API) (*types.DelegationSet, error) {
	// Avoid creating a new delegation set if one already exists
	delegationSet, err := aws.GetDelegationSet(ctx, r53Client)
	// Create a new delegation set if it doesn't exist
	if errors.Is(err, aws.ErrNoDelegationSetFound) {
		// Create a new delegation set. There's a race condition here, where two deployments could create two different delegation sets,
		// but this is acceptable because the next time the zone is deployed, we'll get the existing delegation set from the zone.
		delegationSet, err = aws.CreateDelegationSet(ctx, nil, r53Client)
	}
	if err != nil {
		return nil, err
	}
	return delegationSet, err
}

func getOrCreateDelegationSetByZone(ctx context.Context, zone *types.HostedZone, r53Client aws.Route53API) (*types.DelegationSet, error) {
	projectDomain := dns.Normalize(*zone.Name)
	nsServers, err := aws.ListResourceRecords(ctx, *zone.Id, projectDomain, types.RRTypeNs, r53Client)
	if err != nil {
		return nil, err // TODO: we should not fail deployment if ListResourceRecords fails
	}
	term.Debugf("Zone %q found, NS records: %v", projectDomain, nsServers)

	// Check if the zone was created by the older CLI (delegation set was introduced in v0.6.4)
	var delegationSet *types.DelegationSet
	if zone.Config.Comment != nil && *zone.Config.Comment == aws.CreateHostedZoneCommentLegacy {
		// Case 2a: The zone was created by the older CLI, we'll use the existing NS records; track how many times this happens
		track.Evt("Compose-Up delegateSubdomain old", track.P("domain", projectDomain))

		// Create a dummy delegation set with the existing NS records (but no ID)
		delegationSet = &types.DelegationSet{
			NameServers: nsServers,
		}
	} else {
		// Case 2b or 2c: The zone was not created by an older version of this CLI. We'll get the delegation set and let CD/Pulumi create/update the hosted zone
		// TODO: we need to detect the case 2c where the zone was created by another tool and we need to use the existing NS records

		// Create a reusable delegation set for the existing subdomain zone
		delegationSet, err = aws.CreateDelegationSet(ctx, zone.Id, r53Client)
		if delegationSetAlreadyReusable := new(types.DelegationSetAlreadyReusable); errors.As(err, &delegationSetAlreadyReusable) {
			term.Debug("Route53 delegation set already created:", err)
			delegationSet, err = aws.GetDelegationSetByZone(ctx, zone.Id, r53Client)
		}
		if err != nil {
			return nil, err
		}
	}

	// Ensure the zone's NS records match the ones from the delegation set if the zone already exists
	if !slicesEqualUnordered(delegationSet.NameServers, nsServers) {
		track.Evt("Compose-Up delegateSubdomain diff", track.P("fromDS", delegationSet.NameServers), track.P("fromZone", nsServers))
		term.Debugf("NS records for the existing subdomain zone do not match the delegation set: %v <> %v", delegationSet.NameServers, nsServers)
	}

	return delegationSet, err
}

func slicesEqualUnordered(a, b []string) bool {
	slices.Sort(a)
	slices.Sort(b)
	return slices.Equal(a, b)
}
