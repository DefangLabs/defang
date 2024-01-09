package aws

import (
	"context"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
)

type Region string

type Aws struct {
	Region Region
}

func (r Region) String() string {
	return string(r)
}

func (a Aws) LoadConfig(ctx context.Context) (aws.Config, error) {
	return config.LoadDefaultConfig(ctx, config.WithRegion(string(a.Region)))
}

func GetAccountID(arn string) string {
	parts := strings.Split(arn, ":")
	return parts[4]
}
