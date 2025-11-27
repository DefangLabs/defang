package aws

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws"
	"github.com/DefangLabs/defang/src/pkg/dns"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/aws/smithy-go/ptr"
)

func TestPrepareDomainDelegationMocked(t *testing.T) {
	testPrepareDomainDelegationNew(t, &r53Mock{})
	testPrepareDomainDelegationLegacy(t, &r53Mock{})
}

type r53HostedZone struct {
	types.HostedZone
	types.DelegationSet // no ID => not reusable
}

type route53API interface {
	aws.Route53API
	DeleteHostedZone(ctx context.Context, params *route53.DeleteHostedZoneInput, optFns ...func(*route53.Options)) (*route53.DeleteHostedZoneOutput, error)
}

type r53Mock struct {
	hostedZones    []r53HostedZone
	delegationSets []types.DelegationSet
}

func (r r53Mock) DeleteReusableDelegationSet(ctx context.Context, params *route53.DeleteReusableDelegationSetInput, optFns ...func(*route53.Options)) (*route53.DeleteReusableDelegationSetOutput, error) {
	// TODO: implement if needed
	return nil, nil
}

func (r r53Mock) ListTagsForResource(ctx context.Context, params *route53.ListTagsForResourceInput, optFns ...func(*route53.Options)) (*route53.ListTagsForResourceOutput, error) {
	// TODO: implement if needed
	return nil, nil
}

func (r r53Mock) ListHostedZones(ctx context.Context, params *route53.ListHostedZonesInput, optFns ...func(*route53.Options)) (*route53.ListHostedZonesOutput, error) {
	// TODO: implement if needed
	return nil, nil
}

func (r r53Mock) ListHostedZonesByName(ctx context.Context, params *route53.ListHostedZonesByNameInput, optFns ...func(*route53.Options)) (*route53.ListHostedZonesByNameOutput, error) {
	var hostedZones []types.HostedZone
	for _, hz := range r.hostedZones {
		if params.DNSName != nil && *hz.Name < *params.DNSName { // assume ASCII order
			continue
		}
		hostedZones = append(hostedZones, hz.HostedZone)
		if params.MaxItems != nil && len(hostedZones) >= int(*params.MaxItems) {
			break
		}
	}
	return &route53.ListHostedZonesByNameOutput{
		HostedZones:  hostedZones,
		DNSName:      params.DNSName,
		MaxItems:     params.MaxItems,
		HostedZoneId: params.HostedZoneId,
	}, nil
}

func (r r53Mock) ListResourceRecordSets(ctx context.Context, params *route53.ListResourceRecordSetsInput, optFns ...func(*route53.Options)) (*route53.ListResourceRecordSetsOutput, error) {
	for _, hz := range r.hostedZones {
		if *hz.HostedZone.Id != *params.HostedZoneId {
			continue
		}
		var recordSets []types.ResourceRecord
		if params.StartRecordType == types.RRTypeNs {
			// Copy the NS records from the hosted zone
			for _, ns := range hz.NameServers {
				recordSets = append(recordSets, types.ResourceRecord{Value: ptr.String(ns)})
			}
		}
		return &route53.ListResourceRecordSetsOutput{
			MaxItems: params.MaxItems,
			ResourceRecordSets: []types.ResourceRecordSet{
				{
					Name:            ptr.String(*hz.Name),
					Type:            params.StartRecordType,
					ResourceRecords: recordSets,
				},
			},
		}, nil
	}
	return nil, errors.New("hosted zone not found")
}

func (r *r53Mock) CreateReusableDelegationSet(ctx context.Context, params *route53.CreateReusableDelegationSetInput, optFns ...func(*route53.Options)) (*route53.CreateReusableDelegationSetOutput, error) {
	for _, ds := range r.delegationSets {
		if *ds.CallerReference == *params.CallerReference {
			return nil, &types.DelegationSetAlreadyCreated{}
		}
	}
	var delegationSet *types.DelegationSet
	if params.HostedZoneId != nil {
		for _, hz := range r.hostedZones {
			if strings.HasSuffix(*hz.HostedZone.Id, *params.HostedZoneId) {
				delegationSet = &hz.DelegationSet
				break
			}
		}
		if delegationSet == nil {
			return nil, &types.NoSuchHostedZone{}
		}
		if delegationSet.Id != nil {
			return nil, &types.DelegationSetAlreadyReusable{}
		}
		delegationSet.Id = ptr.String("/delegationset/N" + strings.ToUpper(pkg.RandomID()))
		delegationSet.CallerReference = params.CallerReference
	} else {
		delegationSet = &types.DelegationSet{
			CallerReference: params.CallerReference,
			Id:              ptr.String("/delegationset/N" + strings.ToUpper(pkg.RandomID())),
			NameServers:     []string{r.randNameServer(), r.randNameServer()},
		}
	}
	r.delegationSets = append(r.delegationSets, *delegationSet)
	return &route53.CreateReusableDelegationSetOutput{
		DelegationSet: delegationSet,
		Location:      ptr.String("https://route53.amazonaws.com/2013-04-01" + *delegationSet.Id),
	}, nil
}

func (r r53Mock) ListReusableDelegationSets(ctx context.Context, params *route53.ListReusableDelegationSetsInput, optFns ...func(*route53.Options)) (*route53.ListReusableDelegationSetsOutput, error) {
	return &route53.ListReusableDelegationSetsOutput{
		DelegationSets: r.delegationSets,
		Marker:         params.Marker,
		MaxItems:       params.MaxItems,
	}, nil
}

func (r53Mock) randNameServer() string {
	return pkg.RandomID() + ".example.com"
}

func (r r53Mock) GetHostedZone(ctx context.Context, params *route53.GetHostedZoneInput, optFns ...func(*route53.Options)) (*route53.GetHostedZoneOutput, error) {
	for _, hz := range r.hostedZones {
		if strings.HasSuffix(*hz.HostedZone.Id, *params.Id) {
			return &route53.GetHostedZoneOutput{
				HostedZone:    &hz.HostedZone,
				DelegationSet: &hz.DelegationSet,
			}, nil
		}
	}
	return nil, &types.NoSuchHostedZone{}
}

func (r r53Mock) DeleteHostedZone(ctx context.Context, params *route53.DeleteHostedZoneInput, optFns ...func(*route53.Options)) (*route53.DeleteHostedZoneOutput, error) {
	return &route53.DeleteHostedZoneOutput{}, nil
}

func (r *r53Mock) CreateHostedZone(ctx context.Context, params *route53.CreateHostedZoneInput, optFns ...func(*route53.Options)) (*route53.CreateHostedZoneOutput, error) {
	hostedZone := types.HostedZone{
		Id:              ptr.String("/hostedzone/Z" + strings.ToUpper(pkg.RandomID())),
		CallerReference: params.CallerReference,
		Config:          params.HostedZoneConfig,
		Name:            params.Name,
	}
	var delegationSet *types.DelegationSet
	for _, ds := range r.delegationSets {
		if strings.HasSuffix(*ds.Id, *params.DelegationSetId) {
			delegationSet = &ds
			break
		}
	}
	if delegationSet == nil {
		delegationSet = &types.DelegationSet{
			NameServers: []string{r.randNameServer(), r.randNameServer()},
		}
	}
	r.hostedZones = append(r.hostedZones, r53HostedZone{
		HostedZone:    hostedZone,
		DelegationSet: *delegationSet,
	})
	slices.SortFunc(r.hostedZones, func(a, b r53HostedZone) int {
		return strings.Compare(*a.Name, *b.Name)
	})
	return &route53.CreateHostedZoneOutput{
		DelegationSet: delegationSet,
		HostedZone:    &hostedZone,
		Location:      ptr.String("https://route53.amazonaws.com/2013-04-01" + *hostedZone.Id),
	}, nil
}

func testPrepareDomainDelegationNew(t *testing.T, r53Client route53API) {
	// There's four cases to consider:
	//  2. The subdomain zone exists:
	//    a. DEPRECATED: The zone was created by the older CLI: we need to get the NS records from the existing zone and pass to Fabric; no delegation set
	//    b. The zone was created by the new CD/Pulumi of the same project and stack: we get the create or get the delegation set using the zone
	//    c. The zone was created another way: we ignore it and create a new delegation set and let CD/Pulumi create the hosted zone
	//    d. The zone was created by a different stack: We need to create a new delegation set and let CD/Pulumi create the hosted zone
	ctx := t.Context()

	t.Run("case 1: subdomain zone does not exist", func(t *testing.T) {
		const projectDomain = "byoc.example.internal"
		lookUpCount := 0
		resolverAt := func(nsServer string) dns.Resolver {
			defer func() { lookUpCount++ }()
			switch lookUpCount {
			case 1: // 2nd server in first delegation set contains conflicting NS records
				return dns.MockResolver{Records: map[dns.DNSRequest]dns.DNSResponse{
					{Type: "NS", Domain: projectDomain}: {Records: []string{"ns1.t.net", "ns2.t.net", "ns3.t.net", "ns4.t.net"}, Error: nil},
				}}
			default:
				return dns.MockResolver{Records: map[dns.DNSRequest]dns.DNSResponse{
					{Type: "NS", Domain: projectDomain}: {Records: []string{}, Error: nil},
				}}
			}
		}
		nsServers, delegationSetId, err := prepareDomainDelegation(ctx, projectDomain, "projectname", "stack", r53Client, resolverAt)
		if err != nil {
			t.Fatal(err)
		}
		if len(nsServers) == 0 {
			t.Error("expected name servers")
		}
		if delegationSetId == "" {
			t.Fatal("expected delegation set id")
		}
	})

	// t.Run("case 2a: subdomain zone exists, created by legacy CLI", func(t *testing.T) {
	// 	const projectDomain = "byoc-legacy.example.internal"
	//
	// 	// "Create" the legacy hosted zone
	// 	hz, err := r53Client.CreateHostedZone(ctx, &route53.CreateHostedZoneInput{
	// 		CallerReference: ptr.String(projectDomain + " from testPrepareDomainDelegationLegacy " + pkg.RandomID()),
	// 		Name:            ptr.String(projectDomain),
	// 		HostedZoneConfig: &types.HostedZoneConfig{
	// 			Comment: ptr.String(aws.CreateHostedZoneCommentLegacy),
	// 		},
	// 	})
	// 	if err != nil {
	// 		t.Fatal(err)
	// 	}
	// 	t.Cleanup(func() {
	// 		_, err := r53Client.DeleteHostedZone(ctx, &route53.DeleteHostedZoneInput{
	// 			Id: hz.HostedZone.Id,
	// 		})
	// 		if err != nil {
	// 			t.Error(err)
	// 		}
	// 	})
	//
	// 	nsServers, delegationSetId, err := prepareDomainDelegation(ctx, projectDomain, "projectname", "stack", r53Client, nil)
	// 	if err != nil {
	// 		t.Fatal(err)
	// 	}
	// 	if len(nsServers) == 0 {
	// 		t.Error("expected name servers")
	// 	}
	//
	// 	if !slicesEqualUnordered(nsServers, hz.DelegationSet.NameServers) {
	// 		t.Error("expected same name servers")
	// 	}
	// 	if delegationSetId != "" {
	// 		t.Fatal("expected no delegation set id")
	// 	}
	// })

	// t.Run("reuse existing delegation set", func(t *testing.T) {
	// 	nsServers2, delegationSetId2, err := prepareDomainDelegation(ctx, projectDomain, "projectname", "stack", r53Client, resolverAt)
	// 	if err != nil {
	// 		t.Fatal(err)
	// 	}
	// 	if !slicesEqualUnordered(nsServers, nsServers2) {
	// 		t.Error("expected same name servers")
	// 	}
	// 	if delegationSetId != delegationSetId2 {
	// 		t.Error("expected same delegation set id")
	// 	}
	// })
	//
	// t.Run("reuse existing hosted zone", func(t *testing.T) {
	// 	// Now create the hosted zone like Pulumi would
	// 	hz, err := r53Client.CreateHostedZone(ctx, &route53.CreateHostedZoneInput{
	// 		CallerReference:  ptr.String(projectDomain + " from testPrepareDomainDelegationNew " + pkg.RandomID()),
	// 		Name:             ptr.String(projectDomain),
	// 		DelegationSetId:  ptr.String(delegationSetId),
	// 		HostedZoneConfig: &types.HostedZoneConfig{},
	// 	})
	// 	if err != nil {
	// 		t.Fatal(err)
	// 	}
	// 	t.Cleanup(func() {
	// 		_, err := r53Client.DeleteHostedZone(ctx, &route53.DeleteHostedZoneInput{
	// 			Id: hz.HostedZone.Id,
	// 		})
	// 		if err != nil {
	// 			t.Error(err)
	// 		}
	// 	})
	//
	// 	nsServers2, delegationSetId2, err := prepareDomainDelegation(ctx, projectDomain, "projectname", "stack", r53Client, resolverAt)
	// 	if err != nil {
	// 		t.Fatal(err)
	// 	}
	// 	if !slicesEqualUnordered(nsServers, nsServers2) {
	// 		t.Error("expected same name servers")
	// 	}
	// 	if delegationSetId != delegationSetId2 {
	// 		t.Error("expected same delegation set id")
	// 	}
	// })
}

func testPrepareDomainDelegationLegacy(t *testing.T, r53Client route53API) {
}

// func slicesEqualUnordered(a, b []string) bool {
// 	slices.Sort(a)
// 	slices.Sort(b)
// 	return slices.Equal(a, b)
// }
