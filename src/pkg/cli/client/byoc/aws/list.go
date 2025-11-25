package aws

import (
	"context"
	"fmt"
	"io"
	"iter"
	"strings"
	"sync"

	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws/region"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

func newS3Client(ctx context.Context, region aws.Region) (*s3.Client, error) {
	cfg, err := aws.LoadDefaultConfig(ctx, region)
	if err != nil {
		return nil, AnnotateAwsError(err)
	}

	s3client := s3.NewFromConfig(cfg)
	return s3client, nil
}

func listPulumiStacksInBucket(ctx context.Context, region aws.Region, bucketName string) (iter.Seq2[string, *byoc.PulumiState], error) {
	s3client, err := newS3Client(ctx, region)
	if err != nil {
		return nil, err
	}
	return ListPulumiStacks(ctx, s3client, bucketName)
}

type s3Obj struct{ obj s3types.Object }

func (a s3Obj) Name() string {
	return *a.obj.Key
}

func (a s3Obj) Size() int64 {
	return *a.obj.Size
}

type S3Client interface {
	GetBucketLocation(ctx context.Context, params *s3.GetBucketLocationInput, optFns ...func(*s3.Options)) (*s3.GetBucketLocationOutput, error)
	GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	ListBuckets(ctx context.Context, params *s3.ListBucketsInput, optFns ...func(*s3.Options)) (*s3.ListBucketsOutput, error)
	ListObjectsV2(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
}

func ListPulumiStacks(ctx context.Context, s3client S3Client, bucketName string) (iter.Seq2[string, *byoc.PulumiState], error) {
	prefix := `.pulumi/stacks/` // TODO: should we filter on `projectName`?

	term.Debug("Listing stacks in bucket:", bucketName)
	out, err := s3client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: &bucketName,
		Prefix: &prefix,
	})
	if err != nil {
		return nil, AnnotateAwsError(err)
	}
	return func(yield func(string, *byoc.PulumiState) bool) {
		for _, obj := range out.Contents {
			if obj.Key == nil || obj.Size == nil {
				continue
			}
			stack, state, err := byoc.ParsePulumiStackObject(ctx, s3Obj{obj}, bucketName, prefix, func(ctx context.Context, bucket, path string) ([]byte, error) {
				getObjectOutput, err := s3client.GetObject(ctx, &s3.GetObjectInput{
					Bucket: &bucket,
					Key:    &path,
				})
				if err != nil {
					return nil, err
				}
				return io.ReadAll(getObjectOutput.Body)
			})
			if err != nil {
				term.Debugf("Skipping %q in bucket %s: %v", *obj.Key, bucketName, AnnotateAwsError(err))
				continue
			}
			if stack != "" {
				if !yield(stack, state) {
					break
				}
			}
			// TODO: check for lock files
		}
	}, nil
}

func listPulumiStacksAllRegions(ctx context.Context, s3client S3Client) (iter.Seq2[string, *byoc.PulumiState], error) {
	// Use a single S3 query to list all buckets with the defang-cd- prefix
	// This is faster than calling CloudFormation DescribeStacks in each region
	listBucketsOutput, err := s3client.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		return nil, AnnotateAwsError(err)
	}

	return func(yield func(string, *byoc.PulumiState) bool) {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		type stackAndState struct {
			stack string
			state *byoc.PulumiState
		}
		stackCh := make(chan stackAndState)

		// Filter buckets by prefix and get their locations
		var wg sync.WaitGroup
		for _, bucket := range listBucketsOutput.Buckets {
			if bucket.Name == nil {
				continue
			}
			// Filter by prefix: defang-cd-
			if !strings.HasPrefix(*bucket.Name, byoc.CdTaskPrefix+"-") {
				continue
			}

			// Get bucket location
			locationOutput, err := s3client.GetBucketLocation(ctx, &s3.GetBucketLocationInput{
				Bucket: bucket.Name,
			})
			if err != nil {
				term.Debugf("Skipping bucket %s: failed to get location: %v", *bucket.Name, AnnotateAwsError(err))
				continue
			}

			// GetBucketLocation returns empty string for us-east-1 buckets
			bucketRegion := aws.Region(locationOutput.LocationConstraint)
			if bucketRegion == "" {
				bucketRegion = region.USEast1
			}

			wg.Add(1)
			go func(region aws.Region) {
				defer wg.Done()
				stacks, err := listPulumiStacksInBucket(ctx, region, *bucket.Name)
				if err != nil {
					return
				}
				for stack, state := range stacks {
					select {
					case <-ctx.Done():
						return
					case stackCh <- stackAndState{stack: fmt.Sprintf("%s [%s]", stack, region), state: state}:
					}
				}
			}(bucketRegion)
		}

		go func() {
			wg.Wait()
			close(stackCh) // Close channel when all goroutines are done, which stops the iteration below
		}()

		for stackAndState := range stackCh {
			if !yield(stackAndState.stack, stackAndState.state) {
				break
			}
		}
	}, nil
}
