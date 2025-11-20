package aws

import (
	"context"
	"fmt"
	"io"
	"iter"
	"runtime"
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

func listStacksInRegionWorker(ctx context.Context, jobsCh <-chan region.Region, stackCh chan<- string) {
	for {
		select {
		case <-ctx.Done():
			return
		case region, ok := <-jobsCh:
			if !ok {
				return // no more jobs
			}
			stacks, err := listPulumiStacksInRegion(ctx, region)
			if err != nil {
				term.Debugf("Skipping region %s: %v", region, err)
				continue
			}
			for stack := range stacks {
				select {
				case <-ctx.Done():
					return
				case stackCh <- fmt.Sprintf("%s [%s]", stack, region):
				}
			}
		}
	}
}

func listPulumiStacksInRegionsParallel(ctx context.Context, regions []region.Region) iter.Seq[string] {
	jobsCh := make(chan region.Region, len(regions))
	stackCh := make(chan string)

	return func(yield func(string) bool) {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		// Start N workers
		var wg sync.WaitGroup
		for range runtime.NumCPU() {
			wg.Add(1)
			go func() {
				defer wg.Done()
				listStacksInRegionWorker(ctx, jobsCh, stackCh)
			}()
		}
		// Feed the jobs
		for _, region := range regions {
			jobsCh <- region // non-blocking because of buffered channel
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
