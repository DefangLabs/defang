package aws

import (
	"context"

	rgt "github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	rgttypes "github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi/types"
)

type ResourceGroupsTaggingAPI interface {
	GetResources(ctx context.Context, params *rgt.GetResourcesInput, optFns ...func(*rgt.Options)) (*rgt.GetResourcesOutput, error)
}

// FindResourceArnsByTags returns the ARNs of resources that carry every given tag, scoped to the
// given resource type filters (e.g. "elasticloadbalancing:loadbalancer", "rds:db"). Unlike a name
// guess, this is exact regardless of how the resource happens to be named.
func FindResourceArnsByTags(ctx context.Context, tags map[string]string, resourceTypeFilters []string, svc ResourceGroupsTaggingAPI) ([]string, error) {
	filters := make([]rgttypes.TagFilter, 0, len(tags))
	for key, value := range tags {
		filters = append(filters, rgttypes.TagFilter{Key: &key, Values: []string{value}})
	}

	var arns []string
	var token *string
	for {
		out, err := svc.GetResources(ctx, &rgt.GetResourcesInput{
			TagFilters:          filters,
			ResourceTypeFilters: resourceTypeFilters,
			PaginationToken:     token,
		})
		if err != nil {
			return nil, err
		}
		for _, mapping := range out.ResourceTagMappingList {
			if mapping.ResourceARN != nil {
				arns = append(arns, *mapping.ResourceARN)
			}
		}
		if out.PaginationToken == nil || *out.PaginationToken == "" {
			return arns, nil
		}
		token = out.PaginationToken
	}
}
