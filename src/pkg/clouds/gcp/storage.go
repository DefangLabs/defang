package gcp

import (
	"context"
	"fmt"

	"cloud.google.com/go/storage"
	"github.com/DefangLabs/defang/src/pkg"

	"google.golang.org/api/iterator"
)

func (gcp Gcp) EnsureBucketExists(ctx context.Context, projectID, prefix string) (string, error) {
	existing, err := gcp.GetBucketWithPrefix(ctx, projectID, prefix)
	if err != nil {
		return "", fmt.Errorf("GetBucketWithPrefix: %w", err)
	}
	if existing != "" {
		return existing, nil
	}

	client, err := storage.NewClient(ctx)
	if err != nil {
		return "", fmt.Errorf("storage.NewClient: %w", err)
	}
	defer client.Close()

	newBucketName := fmt.Sprintf("%s-%s", prefix, pkg.RandomID())
	fmt.Println("Creating new bucket:", newBucketName)

	bucket := client.Bucket(newBucketName)
	if err := bucket.Create(ctx, projectID, &storage.BucketAttrs{
		Location:     gcp.Region,
		StorageClass: "STANDARD", // No minimum storage duration
	}); err != nil {
		return "", fmt.Errorf("bucket.Create: %w", err)
	}

	return newBucketName, nil
}

func (gcp Gcp) GetBucketWithPrefix(ctx context.Context, projectID, prefix string) (string, error) {
	client, err := storage.NewClient(ctx)
	if err != nil {
		return "", fmt.Errorf("storage.NewClient: %v", err)
	}
	defer client.Close()

	// List all buckets in the specified project
	it := client.Buckets(ctx, projectID)
	it.Prefix = prefix
	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return "", fmt.Errorf("bucket iterator error: %v", err)
		}

		return attrs.Name, nil
	}

	return "", nil
}
