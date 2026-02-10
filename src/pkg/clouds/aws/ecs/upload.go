package ecs

import (
	"context"
	"errors"
	"regexp"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go/ptr"
	"github.com/google/uuid"
)

// From https://docs.aws.amazon.com/AmazonS3/latest/userguide/object-keys.html
var s3InvalidCharsRegexp = regexp.MustCompile(`[^a-zA-Z0-9!_.*'()-]`)

const prefix = "uploads/"

func (a *AwsEcs) CreateUploadURL(ctx context.Context, name string) (string, error) {
	cfg, err := a.LoadConfigForCD(ctx)
	if err != nil {
		return "", err
	}

	if name == "" {
		name = uuid.NewString()
	} else {
		if len(name) > 64 {
			return "", errors.New("name must be less than 64 characters")
		}
		// Sanitize the digest so it's safe to use as a file name
		name = s3InvalidCharsRegexp.ReplaceAllString(name, "_")
		// name = path.Join(buildsPath, tenantName.String(), digest); TODO: avoid collisions between tenants
	}

	s3Client := s3.NewFromConfig(cfg)
	// Use S3 SDK to create a presigned URL for uploading a file.
	req, err := s3.NewPresignClient(s3Client).PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket: &a.BucketName,
		Key:    ptr.String(prefix + name),
	})
	if err != nil {
		return "", err
	}
	return req.URL, nil
}
