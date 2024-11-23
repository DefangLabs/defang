package gcp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"github.com/DefangLabs/defang/src/pkg"
	"github.com/google/uuid"

	"google.golang.org/api/iterator"
)

func (gcp Gcp) EnsureBucketExists(ctx context.Context, prefix string) (string, error) {
	existing, err := gcp.GetBucketWithPrefix(ctx, prefix)
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

	bucket := client.Bucket(newBucketName)
	if err := bucket.Create(ctx, gcp.ProjectId, &storage.BucketAttrs{
		Location:     gcp.Region,
		StorageClass: "STANDARD", // No minimum storage duration
	}); err != nil {
		return "", fmt.Errorf("bucket.Create: %w", err)
	}

	return newBucketName, nil
}

func (gcp Gcp) GetBucketWithPrefix(ctx context.Context, prefix string) (string, error) {
	client, err := storage.NewClient(ctx)
	if err != nil {
		return "", fmt.Errorf("storage.NewClient: %w", err)
	}
	defer client.Close()

	// List all buckets in the specified project
	it := client.Buckets(ctx, gcp.ProjectId)
	it.Prefix = prefix
	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return "", fmt.Errorf("bucket iterator error: %w", err)
		}

		return attrs.Name, nil
	}

	return "", nil
}

func (gcp Gcp) CreateUploadURL(ctx context.Context, bucketName string, objectName string) (string, error) {
	client, err := storage.NewClient(ctx)
	if err != nil {
		return "", fmt.Errorf("storage.NewClient: %w", err)
	}
	defer client.Close()

	if objectName == "" {
		objectName = uuid.NewString()
	} else { // Sanitize object name according to https://cloud.google.com/storage/docs/objects
		if len(objectName) > 1024 {
			return "", fmt.Errorf("object name %q is too long", objectName)
		}
		if strings.ContainsAny(objectName, "\n\r") {
			return "", fmt.Errorf("object name %q cannot contain newline characters", objectName)
		}
		if strings.HasPrefix(objectName, ".well-known/acme-challenge/") {
			return "", fmt.Errorf("object name %q is reserved", objectName)
		}
		if objectName == "." || objectName == ".." {
			return "", fmt.Errorf("object name %q is reserved", objectName)
		}
	}

	bucket := client.Bucket(bucketName)
	opts := &storage.SignedURLOptions{
		Scheme:  storage.SigningSchemeV4,
		Method:  "PUT",
		Expires: time.Now().Add(15 * time.Minute),
	}

	u, err := bucket.SignedURL(objectName, opts)
	if err != nil {
		return "", fmt.Errorf("Bucket(%q).Object(%q).SignedURL: %w", bucketName, objectName, err)
	}
	return u, nil
}
