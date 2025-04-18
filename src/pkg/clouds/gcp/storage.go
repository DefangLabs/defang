package gcp

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	credentials "cloud.google.com/go/iam/credentials/apiv1"
	"cloud.google.com/go/iam/credentials/apiv1/credentialspb"
	"cloud.google.com/go/storage"
	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/google/uuid"

	"google.golang.org/api/iterator"
)

func (gcp Gcp) EnsureBucketExists(ctx context.Context, prefix string) (string, error) {
	existing, err := gcp.GetBucketWithPrefix(ctx, prefix)
	if err != nil {
		return "", fmt.Errorf("failed to get bucket with prefix %q: %w", prefix, err)
	}
	if existing != "" {
		term.Debugf("Bucket %q already exists\n", existing)
		return existing, nil
	}

	client, err := storage.NewClient(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to create storeage client: %w", err)
	}
	defer client.Close()

	newBucketName := fmt.Sprintf("%s-%s", prefix, pkg.RandomID())
	term.Infof("Creating defang cd bucket %q", newBucketName)

	bucket := client.Bucket(newBucketName)
	if err := bucket.Create(ctx, gcp.ProjectId, &storage.BucketAttrs{
		Location:     gcp.Region,
		StorageClass: "STANDARD", // No minimum storage duration
	}); err != nil {
		return "", fmt.Errorf("failed to create bucket %q: %w", newBucketName, err)
	}

	return newBucketName, nil
}

func (gcp Gcp) GetBucketWithPrefix(ctx context.Context, prefix string) (string, error) {
	client, err := storage.NewClient(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get stoage bucket with prefix %q: %w", prefix, err)
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

func (gcp Gcp) CreateUploadURL(ctx context.Context, bucketName, objectName, serviceAccount string) (string, error) {
	client, err := storage.NewClient(ctx)
	if err != nil {
		return "", fmt.Errorf("unable to create upload URL, failed to create storage client: %w", err)
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
		Scheme:         storage.SigningSchemeV4,
		Method:         "PUT",
		GoogleAccessID: serviceAccount,
		SignBytes: func(b []byte) ([]byte, error) {
			return gcp.SignBytes(ctx, b, serviceAccount)
		},
		Expires: time.Now().Add(15 * time.Minute),
	}

	u, err := bucket.SignedURL(objectName, opts)
	if err != nil {
		return "", fmt.Errorf("Bucket(%q).Object(%q).SignedURL: %w", bucketName, objectName, err)
	}
	return u, nil
}

func (gcp Gcp) SignBytes(ctx context.Context, b []byte, name string) ([]byte, error) {
	credSvc, err := credentials.NewIamCredentialsClient(ctx)
	if err != nil {
		return nil, err
	}

	req := &credentialspb.SignBlobRequest{
		Payload: b,
		Name:    name,
	}
	resp, err := credSvc.SignBlob(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.SignedBlob, err
}

func (gcp Gcp) GetBucketObject(ctx context.Context, bucketName, objectName string) ([]byte, error) {
	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to get bucket object, failed to create storage client: %w", err)
	}
	defer client.Close()

	bucket := client.Bucket(bucketName)
	r, err := bucket.Object(objectName).NewReader(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get bucket object (%q) reader: %w", objectName, err)
	}
	defer r.Close()

	return io.ReadAll(r)
}

func (gcp Gcp) IterateBucketObjects(ctx context.Context, bucketName, prefix string, f func(*storage.ObjectAttrs) error) error {
	client, err := storage.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("unable to iterate on bucket object, failed to create storage client: %w", err)
	}
	defer client.Close()

	bucket := client.Bucket(bucketName)
	it := bucket.Objects(ctx, &storage.Query{Prefix: prefix})
	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to iterate on bucket object %q: %w", bucketName, err)
		}

		if err := f(attrs); err != nil {
			return err
		}
	}
	return nil
}
