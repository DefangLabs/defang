package aws

import (
	"context"
	"errors"
	"iter"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type ErrNoSuchKey = types.NoSuchKey

// Deprecated: use ErrNoSuchKey directly
func IsS3NoSuchKeyError(err error) bool {
	var e *types.NoSuchKey
	return errors.As(err, &e)
}

type S3Lister interface {
	GetBucketLocation(ctx context.Context, params *s3.GetBucketLocationInput, optFns ...func(*s3.Options)) (*s3.GetBucketLocationOutput, error)
	ListBuckets(ctx context.Context, params *s3.ListBucketsInput, optFns ...func(*s3.Options)) (*s3.ListBucketsOutput, error)
}

func ListBucketsByPrefix(ctx context.Context, s3client S3Lister, prefix string) (iter.Seq2[string, Region], error) {
	out, err := s3client.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		return nil, err
	}
	return func(yield func(string, Region) bool) {
		for _, bucket := range out.Buckets {
			if bucket.Name == nil {
				continue
			}
			// Filter by prefix
			if !strings.HasPrefix(*bucket.Name, prefix) {
				continue
			}
			// Get bucket location
			locationOutput, err := s3client.GetBucketLocation(ctx, &s3.GetBucketLocationInput{
				Bucket: bucket.Name,
			})
			if err != nil {
				term.Debugf("Skipping bucket %s: failed to get location: %v", *bucket.Name, err)
				continue
			}
			// GetBucketLocation returns empty LocationConstraint for us-east-1 buckets
			bucketRegion := Region(locationOutput.LocationConstraint)
			if bucketRegion == "" {
				bucketRegion = "us-east-1"
			}
			if !yield(*bucket.Name, bucketRegion) {
				break
			}
		}
	}, nil
}
