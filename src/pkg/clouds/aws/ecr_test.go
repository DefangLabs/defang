package aws

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/ecr"
	ecrtypes "github.com/aws/aws-sdk-go-v2/service/ecr/types"
	"github.com/aws/smithy-go/ptr"
)

type mockECR struct {
	repos        []ecrtypes.Repository
	images       []ecrtypes.ImageIdentifier
	deletedBatch [][]ecrtypes.ImageIdentifier
}

func (m *mockECR) DescribeRepositories(_ context.Context, _ *ecr.DescribeRepositoriesInput, _ ...func(*ecr.Options)) (*ecr.DescribeRepositoriesOutput, error) {
	return &ecr.DescribeRepositoriesOutput{Repositories: m.repos}, nil
}

func (m *mockECR) ListImages(_ context.Context, _ *ecr.ListImagesInput, _ ...func(*ecr.Options)) (*ecr.ListImagesOutput, error) {
	return &ecr.ListImagesOutput{ImageIds: m.images}, nil
}

func (m *mockECR) BatchDeleteImage(_ context.Context, in *ecr.BatchDeleteImageInput, _ ...func(*ecr.Options)) (*ecr.BatchDeleteImageOutput, error) {
	m.deletedBatch = append(m.deletedBatch, in.ImageIds)
	return &ecr.BatchDeleteImageOutput{}, nil
}

func TestFindRepositoriesByPrefix(t *testing.T) {
	svc := &mockECR{repos: []ecrtypes.Repository{
		{RepositoryName: ptr.String("portal-production/kaniko-build")},
		{RepositoryName: ptr.String("other-project/build")},
	}}
	found, err := FindRepositoriesByPrefix(t.Context(), "portal-production/", svc)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || *found[0].RepositoryName != "portal-production/kaniko-build" {
		t.Fatalf("expected only the matching repo, got %+v", found)
	}
}

func TestDeleteImagesBatches(t *testing.T) {
	ids := make([]ecrtypes.ImageIdentifier, 150) // exercise the 100-per-batch chunking
	for i := range ids {
		ids[i] = ecrtypes.ImageIdentifier{ImageDigest: ptr.String("sha256:")}
	}
	svc := &mockECR{}
	if err := DeleteImages(t.Context(), "portal-production/kaniko-build", ids, svc); err != nil {
		t.Fatal(err)
	}
	if len(svc.deletedBatch) != 2 || len(svc.deletedBatch[0]) != 100 || len(svc.deletedBatch[1]) != 50 {
		t.Fatalf("expected batches of 100 and 50, got %v", svc.deletedBatch)
	}
}
