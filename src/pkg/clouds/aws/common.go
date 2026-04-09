package aws

import (
	"context"
	"errors"
	"os/exec"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/tokenstore"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/processcreds"
	r53types "github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

type Region = r53types.VPCRegion

type Aws struct {
	AccountID   string
	Region      Region
	TokenStore  tokenstore.TokenStore
	Credentials aws.CredentialsProvider
}

// func (r Region) String() string {
// 	return string(r)
// }

func (a *Aws) LoadConfig(ctx context.Context) (aws.Config, error) {
	cfg, err := LoadDefaultConfig(ctx, config.WithRegion(string(a.Region)))
	if err != nil {
		return cfg, err
	}
	if cfg.Region == "" {
		return cfg, errors.New("missing AWS region: set AWS_REGION or edit your AWS profile at ~/.aws/config")
	}
	a.Region = Region(cfg.Region)
	// Use OAuth credentials from Login() if available, taking priority over the default chain
	if a.Credentials != nil {
		cfg.Credentials = a.Credentials
	}
	// Get caller identity to determine account ID
	if output, err := NewStsFromConfig(cfg).GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{}); err == nil {
		a.AccountID = *output.Account
	}
	return cfg, err
}

func (a *Aws) MakeRegionalARN(service, resourceId string) string {
	if a.AccountID == "" {
		panic("AWS AccountID must be set to make ARN")
	}
	return MakeARN(
		"aws", // aws-cn, aws-us-gov
		service,
		string(a.Region),
		a.AccountID,
		resourceId,
	)
}

func MakeARN(partition, service, region, accountId, resourceId string) string {
	if partition == "" {
		panic("partition must be set to make ARN")
	}
	if service == "" {
		panic("service must be set to make ARN")
	}
	if resourceId == "" {
		panic("resourceId must be set to make ARN")
	}
	return strings.Join([]string{
		"arn",
		partition,
		service,
		region,    // can be empty, eg. for IAM
		accountId, // can be empty, eg. for S3
		resourceId,
	}, ":")
}

func LoadDefaultConfig(ctx context.Context, optFns ...func(*config.LoadOptions) error) (aws.Config, error) {
	cfg, err := config.LoadDefaultConfig(ctx, optFns...)
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
