package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	elbv2 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
)

var AlbOverride AlbAPI

type AlbAPI interface {
	DescribeLoadBalancers(ctx context.Context, params *elbv2.DescribeLoadBalancersInput, optFns ...func(*elbv2.Options)) (*elbv2.DescribeLoadBalancersOutput, error)
}

func newAlbSvc(cfg aws.Config) AlbAPI {
	var svc AlbAPI = AlbOverride

	if svc == nil {
		svc = elbv2.NewFromConfig(cfg)
	}

	return svc
}

func (a *Aws) GetAlbDnsName(ctx context.Context, name string) (string, error) {
	cfg, err := a.LoadConfig(ctx)
	if err != nil {
		return "", err
	}

	svc := newAlbSvc(cfg)

	resp, err := svc.DescribeLoadBalancers(ctx, &elbv2.DescribeLoadBalancersInput{
		Names: []string{name},
	})
	if err != nil {
		return "", err
	}

	if len(resp.LoadBalancers) == 0 {
		return "", fmt.Errorf("no load balancer found with name %s", name)
	}

	return *resp.LoadBalancers[0].DNSName, nil
}
