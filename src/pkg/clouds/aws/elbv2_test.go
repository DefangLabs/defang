package aws

import (
	"context"
	"testing"

	elbv2 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/aws/smithy-go/ptr"
)

type mockELBv2 struct {
	pages         [][]elbv2types.LoadBalancer
	modifiedAttrs []elbv2types.LoadBalancerAttribute
}

func (m *mockELBv2) DescribeLoadBalancers(_ context.Context, in *elbv2.DescribeLoadBalancersInput, _ ...func(*elbv2.Options)) (*elbv2.DescribeLoadBalancersOutput, error) {
	idx := 0
	if in.Marker != nil {
		idx = int((*in.Marker)[0] - '0')
	}
	out := &elbv2.DescribeLoadBalancersOutput{LoadBalancers: m.pages[idx]}
	if idx+1 < len(m.pages) {
		out.NextMarker = ptr.String(string(rune('0' + idx + 1)))
	}
	return out, nil
}

func (m *mockELBv2) ModifyLoadBalancerAttributes(_ context.Context, in *elbv2.ModifyLoadBalancerAttributesInput, _ ...func(*elbv2.Options)) (*elbv2.ModifyLoadBalancerAttributesOutput, error) {
	m.modifiedAttrs = in.Attributes
	return &elbv2.ModifyLoadBalancerAttributesOutput{}, nil
}

func lb(name string) elbv2types.LoadBalancer {
	return elbv2types.LoadBalancer{LoadBalancerName: ptr.String(name), LoadBalancerArn: ptr.String("arn:" + name)}
}

func TestFindLoadBalancersByPrefix(t *testing.T) {
	svc := &mockELBv2{pages: [][]elbv2types.LoadBalancer{
		{lb("Defang-app-beta-7d0"), lb("other-lb")},
		{lb("Defang-app-beta-abc"), lb("Defang-different-beta")},
	}}
	found, err := FindLoadBalancersByPrefix(t.Context(), "Defang-app-beta", svc)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 2 {
		t.Fatalf("expected 2 matches across pages, got %d", len(found))
	}
}

func TestSetALBDeletionProtection(t *testing.T) {
	svc := &mockELBv2{}
	if err := SetALBDeletionProtection(t.Context(), "arn", false, svc); err != nil {
		t.Fatal(err)
	}
	if len(svc.modifiedAttrs) != 1 || *svc.modifiedAttrs[0].Value != "false" {
		t.Fatalf("expected deletion protection set to false, got %+v", svc.modifiedAttrs)
	}
}
