package aws

import (
	"context"
	"errors"
	"net"
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

type r53HostedZone struct {
	types.HostedZone
	types.DelegationSet // no ID => not reusable
	tags                []types.Tag
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
	switch params.ResourceType {
	case types.TagResourceTypeHostedzone:
		for _, hz := range r.hostedZones {
			if strings.TrimPrefix(*hz.HostedZone.Id, "/hostedzone/") == strings.TrimPrefix(*params.ResourceId, "/hostedzone/") {
				return &route53.ListTagsForResourceOutput{
					ResourceTagSet: &types.ResourceTagSet{
						ResourceType: types.TagResourceTypeHostedzone,
						ResourceId:   params.ResourceId,
						Tags:         hz.tags,
					},
				}, nil
			}
		}
		return nil, errors.New("hosted zone not found")
	default:
		return nil, errors.New("unsupported resource type")
	}
}

func (r *r53Mock) setTagsForHostedZone(hostedZoneId string, tags map[string]string) {
	for i, hz := range r.hostedZones {
		if *hz.HostedZone.Id == hostedZoneId {
			for k, v := range tags {
				r.hostedZones[i].tags = append(r.hostedZones[i].tags, types.Tag{
					Key:   ptr.String(k),
					Value: ptr.String(v),
				})
			}
		}
	}
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

type CallbackMockResolver struct {
	MockResolver dns.MockResolver
	Callback     (func(string))
}

func (r *CallbackMockResolver) LookupNS(ctx context.Context, domain string) ([]*net.NS, error) {
	if r.Callback != nil {
		r.Callback(domain)
	}
	return r.MockResolver.LookupNS(ctx, domain)
}

func (r *CallbackMockResolver) LookupIPAddr(ctx context.Context, domain string) ([]net.IPAddr, error) {
	if r.Callback != nil {
		r.Callback(domain)
	}
	return r.MockResolver.LookupIPAddr(ctx, domain)
}
func (r *CallbackMockResolver) LookupCNAME(ctx context.Context, domain string) (string, error) {
	if r.Callback != nil {
		r.Callback(domain)
	}
	return r.MockResolver.LookupCNAME(ctx, domain)
}

func TestPrepareDomainDelegation(t *testing.T) {
	ctx := t.Context()
	noResultResolver := func(domain string) func(nsServer string) dns.Resolver {
		return func(nsServer string) dns.Resolver {
			return dns.MockResolver{Records: map[dns.DNSRequest]dns.DNSResponse{
				{Type: "NS", Domain: domain}:       {Records: []string{}, Error: nil},
				{Type: "NS", Domain: "defang.app"}: {Records: []string{}, Error: nil},
			}}
		}
	}

	t.Run("case 1: subdomain zone does not exist", func(t *testing.T) {
		r53Client := &r53Mock{}
		const projectDomain = "byoc.example.internal"
		lookUpCount := 0
		resolverAt := func(nsServer string) dns.Resolver {
			defer func() { lookUpCount++ }()
			switch lookUpCount {
			case 1: // 2nd server in first delegation set contains conflicting NS records
				return dns.MockResolver{Records: map[dns.DNSRequest]dns.DNSResponse{
					{Type: "NS", Domain: projectDomain}: {Records: []string{"ns1.t.net", "ns2.t.net", "ns3.t.net", "ns4.t.net"}, Error: nil},
					{Type: "NS", Domain: "defang.app"}:  {Records: []string{}, Error: nil},
				}}
			default:
				return dns.MockResolver{Records: map[dns.DNSRequest]dns.DNSResponse{
					{Type: "NS", Domain: projectDomain}: {Records: []string{}, Error: nil},
					{Type: "NS", Domain: "defang.app"}:  {Records: []string{}, Error: nil},
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

	t.Run("case 2a: subdomain zone exists, created by legacy CLI", func(t *testing.T) {
		r53Client := &r53Mock{}
		const projectDomain = "byoc-legacy.example.internal"

		// "Create" the legacy hosted zone
		hz := createHostedZone(t, r53Client, projectDomain, aws.CreateHostedZoneCommentLegacy, nil)

		nsServers, delegationSetId, err := prepareDomainDelegation(ctx, projectDomain, "projectname", "stack", r53Client, noResultResolver(projectDomain))
		if err != nil {
			t.Fatal(err)
		}
		if len(nsServers) == 0 {
			t.Error("expected name servers")
		}

		// Ignore the delegation set of the legacy hosted zone
		if slicesEqualUnordered(nsServers, hz.DelegationSet.NameServers) {
			t.Errorf("expected different name servers, got the same: %v <=> %v", nsServers, hz.DelegationSet.NameServers)
		}
		if delegationSetId == "" {
			t.Fatal("expected delegation set id, got empty")
		}
	})

	t.Run("case 2b: subdomain zone exist from same project and stack", func(t *testing.T) {
		r53Client := &r53Mock{}
		const projectDomain = "byoc.example.internal"
		nsServers, delegationSetId, err := prepareDomainDelegation(ctx, projectDomain, "projectname", "stack", r53Client, noResultResolver(projectDomain))
		if err != nil {
			t.Fatal(err)
		}
		if len(nsServers) == 0 {
			t.Error("expected name servers")
		}
		if delegationSetId == "" {
			t.Fatal("expected delegation set id")
		}

		// Simulate CD creating the hosted zone using the delegation set
		hz := createHostedZone(t, r53Client, projectDomain, aws.CreateHostedZoneCommentLegacy, &delegationSetId)
		r53Client.setTagsForHostedZone(*hz.HostedZone.Id, map[string]string{"defang:project": "projectname", "defang:stack": "stack"})

		// Now prepare domain delegation again, it should reuse the existing delegation set
		nsServers2, delegationSetId2, err := prepareDomainDelegation(ctx, projectDomain, "projectname", "stack", r53Client, noResultResolver(projectDomain))
		if err != nil {
			t.Fatal(err)
		}
		if !slicesEqualUnordered(nsServers, nsServers2) {
			t.Errorf("expected same name servers, got %v and %v", nsServers, nsServers2)
		}
		if delegationSetId != delegationSetId2 {
			t.Fatalf("expected same delegation set id, got %v and %v", delegationSetId, delegationSetId2)
		}
	})

	t.Run("case 2c: subdomain zone exist created by other tools", func(t *testing.T) {
		r53Client := &r53Mock{}
		const projectDomain = "byoc.example.internal"
		// Simulate creating the hosted zone using other tools
		hz := createHostedZone(t, r53Client, projectDomain, "some other tools", ptr.String("OTHER-TOOL-DS-ID"))
		r53Client.setTagsForHostedZone(*hz.HostedZone.Id, map[string]string{"othertool:project": "projectname", "othertools:stack": "stack"})

		// Now prepare domain delegation again, it should create a new delegation set
		nsServers, delegationSetId, err := prepareDomainDelegation(ctx, projectDomain, "projectname", "stack", r53Client, noResultResolver(projectDomain))
		if err != nil {
			t.Fatal(err)
		}
		if len(nsServers) == 0 {
			t.Error("expected name servers")
		}
		if delegationSetId == "OTHER-TOOL-DS-ID" {
			t.Fatalf("expected different delegation set id, got the same: %v", delegationSetId)
		}
	})

	t.Run("case 2d: subdomain zone exist from different project and stack", func(t *testing.T) {
		r53Client := &r53Mock{}
		const projectDomain = "byoc.example.internal"
		nsServers, delegationSetId, err := prepareDomainDelegation(ctx, projectDomain, "projectname", "stack1", r53Client, noResultResolver(projectDomain))
		if err != nil {
			t.Fatal(err)
		}
		if len(nsServers) == 0 {
			t.Error("expected name servers")
		}
		if delegationSetId == "" {
			t.Fatal("expected delegation set id")
		}

		// Simulate CD creating the hosted zone using the delegation set
		hz := createHostedZone(t, r53Client, projectDomain, aws.CreateHostedZoneCommentLegacy, &delegationSetId)
		r53Client.setTagsForHostedZone(*hz.HostedZone.Id, map[string]string{"defang:project": "projectname", "defang:stack": "stack"})

		// Now prepare domain delegation again, it should create a new delegation set since the stack is different
		nsServers2, delegationSetId2, err := prepareDomainDelegation(ctx, projectDomain, "projectname", "stack2", r53Client, noResultResolver(projectDomain))
		if err != nil {
			t.Fatal(err)
		}
		if slicesEqualUnordered(nsServers, nsServers2) {
			t.Errorf("expected different name servers, got the same: %v", nsServers)
		}
		if delegationSetId == delegationSetId2 {
			t.Fatalf("expected different delegation set id, got the same: %v", delegationSetId)
		}
	})

	t.Run("do not use delegation set with NS server conflicting defang.app", func(t *testing.T) {
		r53Client := &r53Mock{}
		const projectDomain = "byoc.example.internal"
		lookUpCount := 0
		var rejectedNSServers []string
		resolverAt := func(nsServer string) dns.Resolver {
			defer func() { lookUpCount++ }()
			switch {
			case lookUpCount < 4: // First few servers in first delegation set contains conflicting NS records with defang.app
				return &CallbackMockResolver{MockResolver: dns.MockResolver{Records: map[dns.DNSRequest]dns.DNSResponse{
					{Type: "NS", Domain: projectDomain}: {Records: []string{}, Error: nil},            // No conflicts when looking up project domain
					{Type: "NS", Domain: "defang.app"}:  {Records: []string{"ns1.t.net"}, Error: nil}, // Conflict when looking up defang.app
				}},
					Callback: func(domain string) {
						if domain == "defang.app" {
							rejectedNSServers = append(rejectedNSServers, nsServer)
						}
					},
				}
			default:
				return &CallbackMockResolver{MockResolver: dns.MockResolver{Records: map[dns.DNSRequest]dns.DNSResponse{
					{Type: "NS", Domain: projectDomain}: {Records: []string{}, Error: nil},
					{Type: "NS", Domain: "defang.app"}:  {Records: []string{}, Error: nil},
				}}}
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
		for _, ns := range nsServers {
			if slices.Contains(rejectedNSServers, ns) {
				t.Errorf("expected no rejected name servers in final delegation set, but found %q", ns)
			}
		}
	})
}

func createHostedZone(t *testing.T, r53Client route53API, projectDomain, comment string, delegationSetId *string) *route53.CreateHostedZoneOutput {
	hz, err := r53Client.CreateHostedZone(t.Context(), &route53.CreateHostedZoneInput{
		CallerReference: ptr.String(projectDomain + " from " + comment + pkg.RandomID()),
		Name:            ptr.String(projectDomain),
		HostedZoneConfig: &types.HostedZoneConfig{
			Comment: ptr.String(comment),
		},
		DelegationSetId: delegationSetId,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, err := r53Client.DeleteHostedZone(t.Context(), &route53.DeleteHostedZoneInput{
			Id: hz.HostedZone.Id,
		})
		if err != nil {
			t.Error(err)
		}
	})
	return hz
}

func slicesEqualUnordered(a, b []string) bool {
	slices.Sort(a)
	slices.Sort(b)
	return slices.Equal(a, b)
}
