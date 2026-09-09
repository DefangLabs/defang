package aws

import (
	"context"
	"testing"

	rgt "github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	rgttypes "github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi/types"
	"github.com/aws/smithy-go/ptr"
)

type mockResourceGroupsTagging struct {
	pages     [][]rgttypes.ResourceTagMapping
	gotFilter []rgttypes.TagFilter
}

func (m *mockResourceGroupsTagging) GetResources(_ context.Context, in *rgt.GetResourcesInput, _ ...func(*rgt.Options)) (*rgt.GetResourcesOutput, error) {
	m.gotFilter = in.TagFilters
	idx := 0
	if in.PaginationToken != nil {
		idx = int((*in.PaginationToken)[0] - '0')
	}
	out := &rgt.GetResourcesOutput{ResourceTagMappingList: m.pages[idx]}
	if idx+1 < len(m.pages) {
		out.PaginationToken = ptr.String(string(rune('0' + idx + 1)))
	}
	return out, nil
}

func mapping(arn string) rgttypes.ResourceTagMapping {
	return rgttypes.ResourceTagMapping{ResourceARN: ptr.String(arn)}
}

func TestFindResourceArnsByTags(t *testing.T) {
	svc := &mockResourceGroupsTagging{pages: [][]rgttypes.ResourceTagMapping{
		{mapping("arn:a"), mapping("arn:b")},
		{mapping("arn:c")},
	}}
	arns, err := FindResourceArnsByTags(t.Context(), map[string]string{
		"defang:project": "app",
		"defang:stack":   "beta",
	}, []string{"elasticloadbalancing:loadbalancer", "rds:db"}, svc)
	if err != nil {
		t.Fatal(err)
	}
	if len(arns) != 3 {
		t.Fatalf("expected 3 ARNs across pages, got %+v", arns)
	}
	if len(svc.gotFilter) != 2 {
		t.Fatalf("expected one TagFilter per tag, got %+v", svc.gotFilter)
	}
}
