package aws

import (
	"context"
	"errors"
	"strings"

	elbv2 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/aws/smithy-go/ptr"
)

// albDeletionProtectionKey is the load balancer attribute that prevents deletion.
const albDeletionProtectionKey = "deletion_protection.enabled"

type ELBv2API interface {
	DescribeLoadBalancers(ctx context.Context, params *elbv2.DescribeLoadBalancersInput, optFns ...func(*elbv2.Options)) (*elbv2.DescribeLoadBalancersOutput, error)
	ModifyLoadBalancerAttributes(ctx context.Context, params *elbv2.ModifyLoadBalancerAttributesInput, optFns ...func(*elbv2.Options)) (*elbv2.ModifyLoadBalancerAttributesOutput, error)
}

// maxLoadBalancerArnsPerCall is the DescribeLoadBalancers LoadBalancerArns limit (AWS API constraint).
const maxLoadBalancerArnsPerCall = 20

// FindLoadBalancersByArns returns the load balancers identified by the given ARNs. An ARN that no
// longer exists is omitted rather than treated as an error, since tag data can lag a deletion.
func FindLoadBalancersByArns(ctx context.Context, arns []string, svc ELBv2API) ([]elbv2types.LoadBalancer, error) {
	var found []elbv2types.LoadBalancer
	for start := 0; start < len(arns); start += maxLoadBalancerArnsPerCall {
		end := min(start+maxLoadBalancerArnsPerCall, len(arns))
		batch := arns[start:end]
		out, err := svc.DescribeLoadBalancers(ctx, &elbv2.DescribeLoadBalancersInput{LoadBalancerArns: batch})
		if err == nil {
			found = append(found, out.LoadBalancers...)
			continue
		}
		var notFound *elbv2types.LoadBalancerNotFoundException
		if !errors.As(err, &notFound) {
			return nil, err
		}
		// One or more ARNs in this batch no longer exist; a single stale ARN fails the whole
		// batch call, so retry individually rather than lose the rest of the batch.
		for _, arn := range batch {
			single, err := svc.DescribeLoadBalancers(ctx, &elbv2.DescribeLoadBalancersInput{LoadBalancerArns: []string{arn}})
			if err != nil {
				if errors.As(err, &notFound) {
					continue
				}
				return nil, err
			}
			found = append(found, single.LoadBalancers...)
		}
	}
	return found, nil
}

// FindLoadBalancersByPrefix returns the load balancers whose name starts with prefix.
func FindLoadBalancersByPrefix(ctx context.Context, prefix string, svc ELBv2API) ([]elbv2types.LoadBalancer, error) {
	var found []elbv2types.LoadBalancer
	var marker *string
	for {
		out, err := svc.DescribeLoadBalancers(ctx, &elbv2.DescribeLoadBalancersInput{Marker: marker})
		if err != nil {
			return nil, err
		}
		for _, lb := range out.LoadBalancers {
			if lb.LoadBalancerName != nil && strings.HasPrefix(*lb.LoadBalancerName, prefix) {
				found = append(found, lb)
			}
		}
		if out.NextMarker == nil {
			return found, nil
		}
		marker = out.NextMarker
	}
}

// SetALBDeletionProtection enables or disables deletion protection on the load balancer. Disabling
// it lets a subsequent `defang down` (Pulumi) delete the load balancer; it is idempotent, so
// callers need not check the current state first.
func SetALBDeletionProtection(ctx context.Context, lbArn string, enabled bool, svc ELBv2API) error {
	value := "false"
	if enabled {
		value = "true"
	}
	_, err := svc.ModifyLoadBalancerAttributes(ctx, &elbv2.ModifyLoadBalancerAttributesInput{
		LoadBalancerArn: &lbArn,
		Attributes: []elbv2types.LoadBalancerAttribute{
			{Key: ptr.String(albDeletionProtectionKey), Value: &value},
		},
	})
	return err
}
