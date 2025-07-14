package aws

import (
	"context"
	"errors"
	"time"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/dns"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/aws/smithy-go/ptr"
)

var (
	ErrZoneNotFound         = errors.New("the Route53 hosted zone was not found")
	ErrNoRecordFound        = errors.New("no Route53 record found in the hosted zone")
	ErrNoDelegationSetFound = errors.New("no Route53 delegation set found")
)

type Route53API interface {
	CreateHostedZone(ctx context.Context, params *route53.CreateHostedZoneInput, optFns ...func(*route53.Options)) (*route53.CreateHostedZoneOutput, error)
	CreateReusableDelegationSet(ctx context.Context, params *route53.CreateReusableDelegationSetInput, optFns ...func(*route53.Options)) (*route53.CreateReusableDelegationSetOutput, error)
	GetHostedZone(ctx context.Context, params *route53.GetHostedZoneInput, optFns ...func(*route53.Options)) (*route53.GetHostedZoneOutput, error)
	ListReusableDelegationSets(ctx context.Context, params *route53.ListReusableDelegationSetsInput, optFns ...func(*route53.Options)) (*route53.ListReusableDelegationSetsOutput, error)
	ListHostedZonesByName(ctx context.Context, params *route53.ListHostedZonesByNameInput, optFns ...func(*route53.Options)) (*route53.ListHostedZonesByNameOutput, error)
	ListResourceRecordSets(ctx context.Context, params *route53.ListResourceRecordSetsInput, optFns ...func(*route53.Options)) (*route53.ListResourceRecordSetsOutput, error)
}

func CreateDelegationSet(ctx context.Context, zoneId *string, r53 Route53API) (*types.DelegationSet, error) {
	params := &route53.CreateReusableDelegationSetInput{
		CallerReference: ptr.String("Created by Defang CLI " + time.Now().Format(time.RFC3339Nano)),
		HostedZoneId:    zoneId,
	}
	resp, err := r53.CreateReusableDelegationSet(ctx, params)
	if err != nil {
		return nil, err
	}
	return resp.DelegationSet, err
}

func GetDelegationSetByZone(ctx context.Context, zoneId *string, r53 Route53API) (*types.DelegationSet, error) {
	params := &route53.GetHostedZoneInput{
		Id: zoneId,
	}
	resp, err := r53.GetHostedZone(ctx, params)
	if err != nil {
		return nil, err
	}
	return resp.DelegationSet, nil
}

func GetDelegationSet(ctx context.Context, r53 Route53API) (*types.DelegationSet, error) {
	params := &route53.ListReusableDelegationSetsInput{}
	resp, err := r53.ListReusableDelegationSets(ctx, params)
	if err != nil {
		return nil, err
	}
	if len(resp.DelegationSets) == 0 {
		return nil, ErrNoDelegationSetFound
	}
	// Return a random delegation set, to work around the 100 zones-per-delegation-set limit,
	// because we can't easily tell how many zones are using each delegation set.
	return &resp.DelegationSets[pkg.RandomIndex(len(resp.DelegationSets))], nil
}

func GetHostedZoneByName(ctx context.Context, domain string, r53 Route53API) (*types.HostedZone, error) {
	params := &route53.ListHostedZonesByNameInput{
		DNSName:  ptr.String(domain),
		MaxItems: ptr.Int32(1),
	}
	resp, err := r53.ListHostedZonesByName(ctx, params)
	if err != nil {
		return nil, err
	}
	if len(resp.HostedZones) == 0 {
		return nil, ErrZoneNotFound
	}

	// ListHostedZonesByName returns all zones that is after the specified domain, we need to check the domain is the same
	zone := resp.HostedZones[0]
	if !isSameDomain(*zone.Name, domain) {
		return nil, ErrZoneNotFound
	}

	return &zone, nil
}

const CreateHostedZoneCommentLegacy = "Created by defang cli"

// Deprecated: let Pulumi create the hosted zone
func CreateHostedZone(ctx context.Context, domain string, r53 Route53API) (*types.HostedZone, error) {
	params := &route53.CreateHostedZoneInput{
		Name:            ptr.String(domain),
		CallerReference: ptr.String(domain + time.Now().String()),
		HostedZoneConfig: &types.HostedZoneConfig{
			Comment: ptr.String(CreateHostedZoneCommentLegacy),
		},
	}
	resp, err := r53.CreateHostedZone(ctx, params)
	if err != nil {
		return nil, err
	}
	return resp.HostedZone, nil
}

func ListResourceRecords(ctx context.Context, zoneId, recordName string, recordType types.RRType, r53 Route53API) ([]string, error) {
	listInput := &route53.ListResourceRecordSetsInput{
		HostedZoneId:    ptr.String(zoneId),
		StartRecordName: ptr.String(recordName),
		StartRecordType: recordType,
		MaxItems:        ptr.Int32(1),
	}

	listResp, err := r53.ListResourceRecordSets(ctx, listInput)
	if err != nil {
		return nil, err
	}

	if len(listResp.ResourceRecordSets) == 0 {
		return nil, ErrNoRecordFound
	}

	records := listResp.ResourceRecordSets[0].ResourceRecords
	values := make([]string, len(records))
	for i, record := range records {
		values[i] = dns.Normalize(*record.Value)
	}
	return values, nil
}

func isSameDomain(domain1 string, domain2 string) bool {
	return dns.Normalize(domain1) == dns.Normalize(domain2)
}
