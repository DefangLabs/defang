package aws

import (
	"context"
	"errors"
	"os/exec"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/processcreds"
	"github.com/defang-io/defang/src/pkg/types"
)

type Region string

const (
	DockerRegistry    = "docker.io"
	EcrPublicRegistry = "public.ecr.aws"
	ProjectName       = types.ProjectName
)

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
	return parts[4]
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

func PlatformToArchOS(platform string) (string, string) {
	parts := strings.SplitN(platform, "/", 3) // Can be "os/arch/variant" like "linux/arm64/v8"

	if len(parts) == 1 {
		arch := parts[0]
		return normalizedArch(arch), ""
	} else {
		os := parts[0]
		arch := parts[1]
		os = strings.ToUpper(os)
		return normalizedArch(arch), os
	}
}

func normalizedArch(arch string) string {
	// From https://docs.aws.amazon.com/AWSCloudFormation/latest/UserGuide/aws-properties-ecs-taskdefinition-runtimeplatform.html#cfn-ecs-taskdefinition-runtimeplatform-cpuarchitecture
	arch = strings.ToUpper(arch)
	if arch == "AMD64" {
		arch = "X86_64"
	}
	return arch
}
