package aws

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/aws-sdk-go-v2/service/route53/types"
)

var (
	ErrNoZoneFound   = errors.New("no zone found")
	ErrNoRecordFound = errors.New("no record found")
)

func GetZoneIdFromDomain(ctx context.Context, domain string, r53 *route53.Client) (string, error) {
	params := &route53.ListHostedZonesByNameInput{
		DNSName:  aws.String(domain),
		MaxItems: aws.Int32(1),
	}
	resp, err := r53.ListHostedZonesByName(ctx, params)
	if err != nil {
		return "", err
	}
	if len(resp.HostedZones) == 0 {
		return "", ErrNoZoneFound
	}

	// ListHostedZonesByName returns all zones that is after the specified domain, we need to check the domain is the same
	zone := resp.HostedZones[0]
	if !isSameDomain(*zone.Name, domain) {
		return "", ErrNoZoneFound
	}

	return *zone.Id, nil
}

func CreateZone(ctx context.Context, domain string, r53 *route53.Client) (string, error) {
	params := &route53.CreateHostedZoneInput{
		Name:            aws.String(domain),
		CallerReference: aws.String(domain + time.Now().String()),
		HostedZoneConfig: &types.HostedZoneConfig{
			Comment: aws.String("Created by defang cli"),
		},
	}
	resp, err := r53.CreateHostedZone(ctx, params)
	if err != nil {
		return "", err
	}
	return *resp.HostedZone.Id, nil
}

func GetRecordsValue(ctx context.Context, zoneId, name string, recordType types.RRType, r53 *route53.Client) ([]string, error) {
	listInput := &route53.ListResourceRecordSetsInput{
		HostedZoneId:    aws.String(zoneId),
		StartRecordName: aws.String(name),
		StartRecordType: recordType,
		MaxItems:        aws.Int32(1),
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
		values[i] = *record.Value
	}
	return values, nil
}

func isSameDomain(domain1 string, domain2 string) bool {
	return strings.TrimSuffix(domain1, ".") == strings.TrimSuffix(domain2, ".")
}
