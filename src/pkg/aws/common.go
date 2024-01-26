package aws

import (
	"context"
	"errors"
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

func (a *Aws) LoadConfig(ctx context.Context) (aws.Config, error) {
	cfg, err := LoadDefaultConfig(ctx, a.Region)
	if err == nil {
		a.Region = Region(cfg.Region)
	}
	if a.Region == "" {
		return cfg, errors.New("missing AWS region: set AWS_REGION or edit your AWS profile")
	}
	return cfg, err
}

func LoadDefaultConfig(ctx context.Context, region Region) (aws.Config, error) {
	return config.LoadDefaultConfig(ctx, config.WithRegion(string(region)))
}

func GetAccountID(arn string) string {
	parts := strings.Split(arn, ":")
	return parts[4]
}
