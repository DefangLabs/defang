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

// mockELBv2ByArn simulates DescribeLoadBalancers(LoadBalancerArns:...): the real API fails the
// whole call with LoadBalancerNotFoundException if any requested ARN doesn't exist, so this
// mock does the same rather than silently filtering.
type mockELBv2ByArn struct {
	byArn map[string]elbv2types.LoadBalancer
	calls [][]string
}

func (m *mockELBv2ByArn) DescribeLoadBalancers(_ context.Context, in *elbv2.DescribeLoadBalancersInput, _ ...func(*elbv2.Options)) (*elbv2.DescribeLoadBalancersOutput, error) {
	m.calls = append(m.calls, in.LoadBalancerArns)
	var found []elbv2types.LoadBalancer
	for _, arn := range in.LoadBalancerArns {
		lb, ok := m.byArn[arn]
		if !ok {
			return nil, &elbv2types.LoadBalancerNotFoundException{Message: ptr.String(arn)}
		}
		found = append(found, lb)
	}
	return &elbv2.DescribeLoadBalancersOutput{LoadBalancers: found}, nil
}

func (m *mockELBv2ByArn) ModifyLoadBalancerAttributes(_ context.Context, _ *elbv2.ModifyLoadBalancerAttributesInput, _ ...func(*elbv2.Options)) (*elbv2.ModifyLoadBalancerAttributesOutput, error) {
	return &elbv2.ModifyLoadBalancerAttributesOutput{}, nil
}

func TestFindLoadBalancersByArns(t *testing.T) {
	svc := &mockELBv2ByArn{byArn: map[string]elbv2types.LoadBalancer{
		"arn:a": lb("a"),
		"arn:b": lb("b"),
	}}
	found, err := FindLoadBalancersByArns(t.Context(), []string{"arn:a", "arn:b"}, svc)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 2 {
		t.Fatalf("expected 2 load balancers, got %d", len(found))
	}
}

func TestFindLoadBalancersByArns_StaleArnDoesNotHideTheRest(t *testing.T) {
	svc := &mockELBv2ByArn{byArn: map[string]elbv2types.LoadBalancer{
		"arn:a": lb("a"),
		// "arn:stale" deliberately absent: it existed when tagged but was deleted since.
	}}
	found, err := FindLoadBalancersByArns(t.Context(), []string{"arn:a", "arn:stale"}, svc)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || *found[0].LoadBalancerName != "a" {
		t.Fatalf("expected only the live load balancer, got %+v", found)
	}
}

func TestFindLoadBalancersByArns_Empty(t *testing.T) {
	svc := &mockELBv2ByArn{}
	found, err := FindLoadBalancersByArns(t.Context(), nil, svc)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 0 {
		t.Fatalf("expected no load balancers, got %+v", found)
	}
	if len(svc.calls) != 0 {
		t.Fatalf("expected no API calls for an empty ARN list, got %d", len(svc.calls))
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
