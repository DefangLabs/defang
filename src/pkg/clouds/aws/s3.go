package aws

import (
	"context"
	"errors"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type ErrNoSuchKey = types.NoSuchKey

// Deprecated: use ErrNoSuchKey directly
func IsS3NoSuchKeyError(err error) bool {
	var e *types.NoSuchKey
	return errors.As(err, &e)
}

type S3GetObjectAPI interface {
	GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
}

var NewS3FromConfig = func(cfg aws.Config) S3GetObjectAPI {
	return s3.NewFromConfig(cfg)
}

type MockS3ClientAPI struct{}

func (MockS3ClientAPI) GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	return nil, &types.NoSuchKey{Message: aws.String("mock: no such key")}
}
