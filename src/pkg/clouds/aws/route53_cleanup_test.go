package aws

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/aws/smithy-go/ptr"
)

// fakeR53 embeds Route53API and implements only the methods exercised by cleanup.
type fakeR53 struct {
	Route53API
	pages   [][]types.ResourceRecordSet
	deleted []types.ResourceRecordSet
}

func (f *fakeR53) ListResourceRecordSets(_ context.Context, in *route53.ListResourceRecordSetsInput, _ ...func(*route53.Options)) (*route53.ListResourceRecordSetsOutput, error) {
	idx := 0
	if in.StartRecordName != nil {
		idx = int((*in.StartRecordName)[0] - '0')
	}
	out := &route53.ListResourceRecordSetsOutput{ResourceRecordSets: f.pages[idx]}
	if idx+1 < len(f.pages) {
		out.IsTruncated = true
		out.NextRecordName = ptr.String(string(rune('0' + idx + 1)))
		out.NextRecordType = types.RRTypeA
	}
	return out, nil
}

func (f *fakeR53) ChangeResourceRecordSets(_ context.Context, in *route53.ChangeResourceRecordSetsInput, _ ...func(*route53.Options)) (*route53.ChangeResourceRecordSetsOutput, error) {
	for _, c := range in.ChangeBatch.Changes {
		f.deleted = append(f.deleted, *c.ResourceRecordSet)
	}
	return &route53.ChangeResourceRecordSetsOutput{}, nil
}

func rrset(name string, t types.RRType) types.ResourceRecordSet {
	return types.ResourceRecordSet{Name: ptr.String(name), Type: t}
}

func TestListAllResourceRecordSetsPaginates(t *testing.T) {
	f := &fakeR53{pages: [][]types.ResourceRecordSet{
		{rrset("a.example.com.", types.RRTypeA)},
		{rrset("b.example.com.", types.RRTypeCname)},
	}}
	records, err := ListAllResourceRecordSets(t.Context(), "Z123", f)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 records across pages, got %d", len(records))
	}
}

func TestDeleteResourceRecordSet(t *testing.T) {
	f := &fakeR53{}
	rec := rrset("app.example.com.", types.RRTypeA)
	if err := DeleteResourceRecordSet(t.Context(), "/hostedzone/Z123", rec, f); err != nil {
		t.Fatal(err)
	}
	if len(f.deleted) != 1 || *f.deleted[0].Name != "app.example.com." {
		t.Fatalf("expected the record to be deleted, got %+v", f.deleted)
	}
}
