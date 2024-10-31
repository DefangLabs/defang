package aws

import (
	"errors"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type ErrNoSuchKey = types.NoSuchKey

// Deprecated: use ErrNoSuchKey directly
func IsS3NoSuchKeyError(err error) bool {
	var e *types.NoSuchKey
	return errors.As(err, &e)
}
