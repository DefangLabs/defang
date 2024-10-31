package aws

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/aws/smithy-go/ptr"
)

var (
	ErrZoneNotFound         = errors.New("the Route53 hosted zone was not found")
	ErrNoRecordFound        = errors.New("no Route53 record found in the hosted zone")
	ErrNoDelegationSetFound = errors.New("no Route53 delegation set found")
)

func CreateDelegationSet(ctx context.Context, zoneId *string, r53 *route53.Client) (*types.DelegationSet, error) {
	params := &route53.CreateReusableDelegationSetInput{
		CallerReference: ptr.String("Created by Defang CLI" + time.Now().String()),
		HostedZoneId:    zoneId,
	}
	resp, err := r53.CreateReusableDelegationSet(ctx, params)
	if err != nil {
		return nil, err
	}
	return resp.DelegationSet, err
}

func GetDelegationSet(ctx context.Context, r53 *route53.Client) (*types.DelegationSet, error) {
	params := &route53.ListReusableDelegationSetsInput{
		MaxItems: ptr.Int32(1),
	}
	resp, err := r53.ListReusableDelegationSets(ctx, params)
	if err != nil {
		return nil, err
	}
	if len(resp.DelegationSets) == 0 {
		return nil, ErrNoDelegationSetFound
	}
	return &resp.DelegationSets[0], nil
}

func GetHostedZoneByName(ctx context.Context, domain string, r53 *route53.Client) (*types.HostedZone, error) {
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

const CreateHostedZoneComment = "Created by defang cli"

// Deprecated: let Pulumi create the hosted zone
func CreateHostedZone(ctx context.Context, domain string, r53 *route53.Client) (*types.HostedZone, error) {
	params := &route53.CreateHostedZoneInput{
		Name:            ptr.String(domain),
		CallerReference: ptr.String(domain + time.Now().String()),
		HostedZoneConfig: &types.HostedZoneConfig{
			Comment: ptr.String(CreateHostedZoneComment),
		},
	}
	resp, err := r53.CreateHostedZone(ctx, params)
	if err != nil {
		return nil, err
	}
	return resp.HostedZone, nil
}

func ListResourceRecords(ctx context.Context, zoneId, recordName string, recordType types.RRType, r53 *route53.Client) ([]string, error) {
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
		values[i] = strings.TrimSuffix(*record.Value, ".") // normalize the value
	}
	return values, nil
}

func isSameDomain(domain1 string, domain2 string) bool {
	return strings.TrimSuffix(domain1, ".") == strings.TrimSuffix(domain2, ".")
}
