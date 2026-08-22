package aws

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/ecr"
	ecrtypes "github.com/aws/aws-sdk-go-v2/service/ecr/types"
)

type ECRAPI interface {
	DescribeRepositories(ctx context.Context, params *ecr.DescribeRepositoriesInput, optFns ...func(*ecr.Options)) (*ecr.DescribeRepositoriesOutput, error)
	ListImages(ctx context.Context, params *ecr.ListImagesInput, optFns ...func(*ecr.Options)) (*ecr.ListImagesOutput, error)
	BatchDeleteImage(ctx context.Context, params *ecr.BatchDeleteImageInput, optFns ...func(*ecr.Options)) (*ecr.BatchDeleteImageOutput, error)
}

// FindRepositoriesByPrefix returns the ECR repositories whose name starts with prefix.
func FindRepositoriesByPrefix(ctx context.Context, prefix string, svc ECRAPI) ([]ecrtypes.Repository, error) {
	var found []ecrtypes.Repository
	var token *string
	for {
		out, err := svc.DescribeRepositories(ctx, &ecr.DescribeRepositoriesInput{NextToken: token})
		if err != nil {
			return nil, err
		}
		for _, repo := range out.Repositories {
			if repo.RepositoryName != nil && strings.HasPrefix(*repo.RepositoryName, prefix) {
				found = append(found, repo)
			}
		}
		if out.NextToken == nil {
			return found, nil
		}
		token = out.NextToken
	}
}

// ListImageIDs returns the identifiers of every image in the repository.
func ListImageIDs(ctx context.Context, repoName string, svc ECRAPI) ([]ecrtypes.ImageIdentifier, error) {
	var ids []ecrtypes.ImageIdentifier
	var token *string
	for {
		out, err := svc.ListImages(ctx, &ecr.ListImagesInput{RepositoryName: &repoName, NextToken: token})
		if err != nil {
			return nil, err
		}
		ids = append(ids, out.ImageIds...)
		if out.NextToken == nil {
			return ids, nil
		}
		token = out.NextToken
	}
}

// DeleteImages removes the given images from the repository, batching the calls (BatchDeleteImage
// accepts at most 100 image IDs). Emptying the repository lets a subsequent `defang down` (Pulumi)
// delete it, which otherwise fails with RepositoryNotEmptyException.
func DeleteImages(ctx context.Context, repoName string, ids []ecrtypes.ImageIdentifier, svc ECRAPI) error {
	for start := 0; start < len(ids); start += 100 {
		end := min(start+100, len(ids))
		out, err := svc.BatchDeleteImage(ctx, &ecr.BatchDeleteImageInput{
			RepositoryName: &repoName,
			ImageIds:       ids[start:end],
		})
		if err != nil {
			return err
		}
		// BatchDeleteImage succeeds (HTTP 200) even when individual images fail to delete, and a
		// single leftover image is enough to keep blocking the repository's deletion.
		if err := batchDeleteFailure(repoName, out.Failures); err != nil {
			return err
		}
	}
	return nil
}

// batchDeleteFailure summarizes the per-image failures of a BatchDeleteImage response, or returns
// nil if every image was deleted.
func batchDeleteFailure(repoName string, failures []ecrtypes.ImageFailure) error {
	if len(failures) == 0 {
		return nil
	}
	reasons := make([]string, 0, len(failures))
	for _, f := range failures {
		ref := "<unknown>"
		if f.ImageId != nil {
			if f.ImageId.ImageTag != nil {
				ref = *f.ImageId.ImageTag
			} else if f.ImageId.ImageDigest != nil {
				ref = *f.ImageId.ImageDigest
			}
		}
		reason := string(f.FailureCode)
		if f.FailureReason != nil {
			reason = *f.FailureReason
		}
		reasons = append(reasons, ref+": "+reason)
	}
	return fmt.Errorf("failed to delete %d image(s) from repository %s: %s", len(failures), repoName, strings.Join(reasons, "; "))
}
