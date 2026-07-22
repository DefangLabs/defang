package cfn

import (
	"context"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/clouds/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go/ptr"
)

// mockBucketLister implements bucketLister. All buckets are reported in `region`,
// and tags are looked up per bucket name from `tags`.
type mockBucketLister struct {
	buckets []string
	region  string
	tags    map[string][]s3types.Tag
}

func (m mockBucketLister) ListBuckets(context.Context, *s3.ListBucketsInput, ...func(*s3.Options)) (*s3.ListBucketsOutput, error) {
	out := &s3.ListBucketsOutput{}
	for _, n := range m.buckets {
		out.Buckets = append(out.Buckets, s3types.Bucket{Name: ptr.String(n)})
	}
	return out, nil
}

func (m mockBucketLister) GetBucketLocation(context.Context, *s3.GetBucketLocationInput, ...func(*s3.Options)) (*s3.GetBucketLocationOutput, error) {
	return &s3.GetBucketLocationOutput{LocationConstraint: s3types.BucketLocationConstraint(m.region)}, nil
}

func (m mockBucketLister) GetBucketTagging(_ context.Context, in *s3.GetBucketTaggingInput, _ ...func(*s3.Options)) (*s3.GetBucketTaggingOutput, error) {
	return &s3.GetBucketTaggingOutput{TagSet: m.tags[*in.Bucket]}, nil
}

func tagSet(prefixValue string) []s3types.Tag {
	return []s3types.Tag{{Key: ptr.String(TagKeyPrefix), Value: ptr.String(prefixValue)}}
}

func TestFindAdoptableBucket(t *testing.T) {
	const stack = "defang-cd"
	const region = aws.Region("us-west-2")

	tests := []struct {
		name    string
		lister  mockBucketLister
		want    string
		wantErr bool
	}{
		{
			name:   "none",
			lister: mockBucketLister{buckets: []string{"unrelated-bucket"}, region: "us-west-2"},
			want:   "",
		},
		{
			name: "one match adopted",
			lister: mockBucketLister{
				buckets: []string{"defang-cd-bucket-aaa", "some-other-bucket"},
				region:  "us-west-2",
				tags:    map[string][]s3types.Tag{"defang-cd-bucket-aaa": tagSet(stack)},
			},
			want: "defang-cd-bucket-aaa",
		},
		{
			name: "prefix match but wrong region is skipped",
			lister: mockBucketLister{
				buckets: []string{"defang-cd-bucket-aaa"},
				region:  "us-east-1",
				tags:    map[string][]s3types.Tag{"defang-cd-bucket-aaa": tagSet(stack)},
			},
			want: "",
		},
		{
			name: "prefix match but not our tag is skipped",
			lister: mockBucketLister{
				buckets: []string{"defang-cd-bucket-aaa"},
				region:  "us-west-2",
				tags:    map[string][]s3types.Tag{"defang-cd-bucket-aaa": tagSet("someone-else")},
			},
			want: "",
		},
		{
			name: "multiple matches is ambiguous",
			lister: mockBucketLister{
				buckets: []string{"defang-cd-bucket-aaa", "defang-cd-bucket-bbb"},
				region:  "us-west-2",
				tags: map[string][]s3types.Tag{
					"defang-cd-bucket-aaa": tagSet(stack),
					"defang-cd-bucket-bbb": tagSet(stack),
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := findAdoptableBucket(context.Background(), tt.lister, stack, region)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (result %q)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("findAdoptableBucket() = %q, want %q", got, tt.want)
			}
		})
	}
}
