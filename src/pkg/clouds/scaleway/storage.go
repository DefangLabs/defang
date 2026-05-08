package scaleway

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3Endpoint returns the S3-compatible endpoint for a Scaleway region.
func S3Endpoint(region Region) string {
	return fmt.Sprintf("https://s3.%s.scw.cloud", region)
}

// NewS3Client creates an AWS S3 client configured for Scaleway Object Storage.
func NewS3Client(region Region, accessKey, secretKey string) *s3.Client {
	return s3.New(s3.Options{
		Region:       region,
		BaseEndpoint: aws.String(S3Endpoint(region)),
		Credentials:  credentials.NewStaticCredentialsProvider(accessKey, secretKey, ""),
		UsePathStyle: true,
	})
}

// EnsureBucketExists creates the bucket if it does not already exist.
func EnsureBucketExists(ctx context.Context, client *s3.Client, bucketName, region string) error {
	_, err := client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(bucketName),
	})
	if err == nil {
		return nil // bucket already exists
	}

	_, err = client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(bucketName),
	})
	if err != nil {
		return fmt.Errorf("creating bucket %q: %w", bucketName, err)
	}
	return nil
}

// CreatePresignedUploadURL generates a presigned PUT URL for uploading an object.
func CreatePresignedUploadURL(ctx context.Context, client *s3.Client, bucket, key string, expiry time.Duration) (string, error) {
	presignClient := s3.NewPresignClient(client)
	req, err := presignClient.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(expiry))
	if err != nil {
		return "", fmt.Errorf("creating presigned upload URL: %w", err)
	}
	return req.URL, nil
}

// GetObject retrieves an object from S3-compatible storage.
func GetObject(ctx context.Context, client *s3.Client, bucket, key string) ([]byte, error) {
	resp, err := client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("getting object %q from bucket %q: %w", key, bucket, err)
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

// PutObject uploads an object to S3-compatible storage.
func PutObject(ctx context.Context, client *s3.Client, bucket, key string, body io.Reader) error {
	_, err := client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Body:   body,
	})
	if err != nil {
		return fmt.Errorf("putting object %q to bucket %q: %w", key, bucket, err)
	}
	return nil
}

// ListObjectKeys lists object keys in a bucket with an optional prefix.
func ListObjectKeys(ctx context.Context, client *s3.Client, bucket, prefix string) ([]string, error) {
	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
	}
	if prefix != "" {
		input.Prefix = aws.String(prefix)
	}

	var keys []string
	paginator := s3.NewListObjectsV2Paginator(client, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("listing objects in bucket %q: %w", bucket, err)
		}
		for _, obj := range page.Contents {
			keys = append(keys, aws.ToString(obj.Key))
		}
	}
	return keys, nil
}
