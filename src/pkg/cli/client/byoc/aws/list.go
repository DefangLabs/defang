package aws

import (
	"context"
	"fmt"
	"io"
	"iter"
	"runtime"
	"strings"
	"sync"

	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws/ecs/cfn"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws/region"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

func listPulumiStacksInRegion(ctx context.Context, region aws.Region) (iter.Seq[string], error) {
	// Determine the bucket name from the CloudFormation stack outputs (if any); this is slow!
	driver := cfn.New(byoc.CdTaskPrefix, region)
	if err := driver.FillOutputs(ctx); err != nil {
		return nil, err
	}
	return listPulumiStacksInBucket(ctx, region, driver.BucketName)
}

func listPulumiStacksInBucket(ctx context.Context, region aws.Region, bucketName string) (iter.Seq[string], error) {
	cfg, err := aws.LoadDefaultConfig(ctx, region)
	if err != nil {
		return nil, AnnotateAwsError(err)
	}

	s3client := s3.NewFromConfig(cfg)
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
	GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	ListObjectsV2(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
}

func ListPulumiStacks(ctx context.Context, s3client S3Client, bucketName string) (iter.Seq[string], error) {
	prefix := `.pulumi/stacks/` // TODO: should we filter on `projectName`?

	term.Debug("Listing stacks in bucket:", bucketName)
	out, err := s3client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: &bucketName,
		Prefix: &prefix,
	})
	if err != nil {
		return nil, AnnotateAwsError(err)
	}
	return func(yield func(string) bool) {
		for _, obj := range out.Contents {
			if obj.Key == nil || obj.Size == nil {
				continue
			}
			stack, err := byoc.ParsePulumiStackObject(ctx, s3Obj{obj}, bucketName, prefix, func(ctx context.Context, bucket, path string) ([]byte, error) {
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
				if !yield(stack) {
					break
				}
			}
			// TODO: check for lock files
		}
	}, nil
}

type bucketInfo struct {
	name   string
	region aws.Region
}

func listStacksInBucketWorker(ctx context.Context, jobsCh <-chan bucketInfo, stackCh chan<- string) {
	for {
		select {
		case <-ctx.Done():
			return
		case bucket, ok := <-jobsCh:
			if !ok {
				return // no more jobs
			}
			stacks, err := listPulumiStacksInBucket(ctx, bucket.region, bucket.name)
			if err != nil {
				term.Debugf("Skipping bucket %s: %v", bucket.name, err)
				continue
			}
			for stack := range stacks {
				select {
				case <-ctx.Done():
					return
				case stackCh <- fmt.Sprintf("%s [%s]", stack, bucket.region):
				}
			}
		}
	}
}

func listPulumiStacksInRegionsParallel(ctx context.Context, regions []region.Region) iter.Seq[string] {
	return func(yield func(string) bool) {
		// Use a single S3 query to list all buckets with the defang-cd- prefix
		// This is faster than calling CloudFormation DescribeStacks in each region
		// Note: S3 ListBuckets is a global operation, so we use empty region
		cfg, err := aws.LoadDefaultConfig(ctx, "")
		if err != nil {
			term.Debugf("Failed to load AWS config: %v", AnnotateAwsError(err))
			return
		}

		s3client := s3.NewFromConfig(cfg)
		listBucketsOutput, err := s3client.ListBuckets(ctx, &s3.ListBucketsInput{})
		if err != nil {
			term.Debugf("Failed to list S3 buckets: %v", AnnotateAwsError(err))
			return
		}

		// Filter buckets by prefix and get their locations
		var buckets []bucketInfo
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
			bucketRegion := string(locationOutput.LocationConstraint)
			if bucketRegion == "" {
				bucketRegion = string(region.USEast1)
			}

			buckets = append(buckets, bucketInfo{
				name:   *bucket.Name,
				region: aws.Region(bucketRegion),
			})
		}

		if len(buckets) == 0 {
			return
		}

		// Process buckets in parallel
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		jobsCh := make(chan bucketInfo, len(buckets))
		stackCh := make(chan string)

		// Start N workers
		var wg sync.WaitGroup
		for range runtime.NumCPU() {
			wg.Add(1)
			go func() {
				defer wg.Done()
				listStacksInBucketWorker(ctx, jobsCh, stackCh)
			}()
		}

		// Feed the jobs
		for _, bucket := range buckets {
			jobsCh <- bucket // non-blocking because of buffered channel
		}
		close(jobsCh)

		go func() {
			wg.Wait()
			close(stackCh) // stops the consumer for loop
		}()

		for stack := range stackCh {
			if !yield(stack) {
				break
			}
		}
	}
}
