package aws

import (
	"context"
	"testing"

	"github.com/DefangLabs/defang/src/pkg"
	elbv2 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"

	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
)

type mockAlbAPI struct {
	dnsName string
}

func (m *mockAlbAPI) DescribeLoadBalancers(ctx context.Context, params *elbv2.DescribeLoadBalancersInput, optFns ...func(*elbv2.Options)) (*elbv2.DescribeLoadBalancersOutput, error) {
	output := &elbv2.DescribeLoadBalancersOutput{}
	if m.dnsName != "" {
		output.LoadBalancers = []elbv2types.LoadBalancer{
			{
				DNSName: &m.dnsName,
			},
		}
	}
	return output, nil
}

func TestGetAlbDnsName(t *testing.T) {
	a := Aws{Region: Region(pkg.Getenv("AWS_REGION", "us-west-2"))}
	t.Run("Should report correct dns name", func(t *testing.T) {
		expectedDnsName := "example.com"
		AlbOverride = &mockAlbAPI{dnsName: expectedDnsName}

		dns, err := a.GetAlbDnsName(context.Background(), "test")
		if err != nil {
			t.Errorf("expected nil, got %v", err)
		}
		if dns != expectedDnsName {
			t.Errorf("expected %s, got %s", expectedDnsName, dns)
		}
	})

	t.Run("No dns name found should result in an errer", func(t *testing.T) {
		AlbOverride = &mockAlbAPI{}

		dns, err := a.GetAlbDnsName(context.Background(), "test")
		if err == nil {
			t.Errorf("expected error, got nil")
		}
		if err.Error() != "no load balancer found with name test" {
			t.Errorf("expected no load balancer found with name test, got %s", err.Error())
		}
		if dns != "" {
			t.Errorf("expected empty string, got %s", dns)
		}
	})
}
