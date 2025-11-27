package aws

import (
	"context"
	"errors"
	"strings"
	"time"

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
	DeleteReusableDelegationSet(ctx context.Context, params *route53.DeleteReusableDelegationSetInput, optFns ...func(*route53.Options)) (*route53.DeleteReusableDelegationSetOutput, error)
	GetHostedZone(ctx context.Context, params *route53.GetHostedZoneInput, optFns ...func(*route53.Options)) (*route53.GetHostedZoneOutput, error)
	ListReusableDelegationSets(ctx context.Context, params *route53.ListReusableDelegationSetsInput, optFns ...func(*route53.Options)) (*route53.ListReusableDelegationSetsOutput, error)

	ListHostedZones(ctx context.Context, params *route53.ListHostedZonesInput, optFns ...func(*route53.Options)) (*route53.ListHostedZonesOutput, error)
	ListHostedZonesByName(ctx context.Context, params *route53.ListHostedZonesByNameInput, optFns ...func(*route53.Options)) (*route53.ListHostedZonesByNameOutput, error)
	ListResourceRecordSets(ctx context.Context, params *route53.ListResourceRecordSetsInput, optFns ...func(*route53.Options)) (*route53.ListResourceRecordSetsOutput, error)
	ListTagsForResource(ctx context.Context, params *route53.ListTagsForResourceInput, optFns ...func(*route53.Options)) (*route53.ListTagsForResourceOutput, error)
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

func DeleteDelegationSet(ctx context.Context, delegationSetId *string, r53 Route53API) error {
	params := &route53.DeleteReusableDelegationSetInput{
		Id: delegationSetId,
	}
	_, err := r53.DeleteReusableDelegationSet(ctx, params)
	return err
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

func GetHostedZonesByName(ctx context.Context, domain string, r53 Route53API) ([]*types.HostedZone, error) {
	var nextHostedZoneId *string
	var zones []*types.HostedZone
	for {
		params := &route53.ListHostedZonesByNameInput{
			DNSName:      &domain,
			HostedZoneId: nextHostedZoneId,
		}
		resp, err := r53.ListHostedZonesByName(ctx, params)
		if err != nil {
			return nil, err
		}

		for _, zone := range resp.HostedZones {
			// ListHostedZonesByName returns all zones that is after the specified domain, we need to check the domain is the same
			if isSameDomain(*zone.Name, domain) {
				zones = append(zones, &zone)
			} else {
				// Since the zones are returned in alphabetical order, we can stop searching once we find a zone that does not match
				break
			}
		}
		// Ignore subsequent zones that do not exactly match the domain, as the zones are returned alphabetically
		if !resp.IsTruncated || (resp.NextDNSName != nil && !isSameDomain(*resp.NextDNSName, domain)) {
			break
		}
		nextHostedZoneId = resp.NextHostedZoneId
	}

	if len(zones) == 0 {
		return nil, ErrZoneNotFound
	}
	return zones, nil
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

func GetHostedZoneTags(ctx context.Context, zoneId string, r53 Route53API) (map[string]string, error) {
	zoneId = strings.TrimPrefix(zoneId, "/hostedzone/")
	listResp, err := r53.ListTagsForResource(ctx, &route53.ListTagsForResourceInput{
		ResourceType: types.TagResourceTypeHostedzone,
		ResourceId:   ptr.String(zoneId),
	})
	if err != nil {
		return nil, err
	}

	if listResp == nil || listResp.ResourceTagSet == nil {
		return nil, nil
	}

	tags := make(map[string]string)
	for _, tag := range listResp.ResourceTagSet.Tags {
		if tag.Key != nil {
			value := ""
			if tag.Value != nil {
				value = *tag.Value
			}
			tags[*tag.Key] = value
		}
	}
	return tags, nil
}

func isSameDomain(domain1 string, domain2 string) bool {
	return dns.Normalize(domain1) == dns.Normalize(domain2)
}
