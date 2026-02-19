package aws

import (
	"context"
	"errors"
	"os/exec"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/processcreds"
	r53types "github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

type Region = r53types.VPCRegion

type Aws struct {
	AccountID string
	Region    Region
}

// func (r Region) String() string {
// 	return string(r)
// }

func (a *Aws) LoadConfig(ctx context.Context) (aws.Config, error) {
	cfg, err := LoadDefaultConfig(ctx, a.Region)
	if err != nil {
		return cfg, err
	}
	if cfg.Region == "" {
		return cfg, errors.New("missing AWS region: set AWS_REGION or edit your AWS profile at ~/.aws/config")
	}
	a.Region = Region(cfg.Region)
	// Get caller identity to determine account ID
	if output, err := NewStsFromConfig(cfg).GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{}); err == nil {
		a.AccountID = *output.Account
	}
	return cfg, err
}

func LoadDefaultConfig(ctx context.Context, region Region) (aws.Config, error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(string(region)))
	if err != nil {
		return cfg, err
	}

	// cliProvider invokes aws cli to obtain credentials aws cli is using
	// Based on https://github.com/aws/aws-sdk-go-v2/issues/1433
	cliProvider := processcreds.NewProviderCommand(
		processcreds.NewCommandBuilderFunc(
			func(ctx context.Context) (*exec.Cmd, error) {
				return exec.CommandContext(ctx, "aws", "configure", "export-credentials", "--format", "process"), nil
			},
		),
	)

	cfg.Credentials = newChainProvider(
		cliProvider,
		cfg.Credentials,
	)
	return cfg, nil
}

func GetAccountID(arn string) string {
	parts := strings.Split(arn, ":")
	return parts[4] // panics if the ARN is malformed
}

func newChainProvider(providers ...aws.CredentialsProvider) aws.CredentialsProvider {
	return aws.NewCredentialsCache(
		aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
			var errs []error

			for _, p := range providers {
				creds, err := p.Retrieve(ctx)
				if err == nil {
					return creds, nil
				}

				errs = append(errs, err)
			}

			return aws.Credentials{}, errors.Join(errs...)
		}),
	)
}
