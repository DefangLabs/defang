package aws

import (
	"context"
	"errors"
	"sort"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/aws/smithy-go/ptr"
)

type SsmParametersAPI interface {
	DescribeParameters(ctx context.Context, params *ssm.DescribeParametersInput, optFns ...func(*ssm.Options)) (*ssm.DescribeParametersOutput, error)
	PutParameter(ctx context.Context, params *ssm.PutParameterInput, optFns ...func(*ssm.Options)) (*ssm.PutParameterOutput, error)
	DeleteParameters(ctx context.Context, params *ssm.DeleteParametersInput, optFns ...func(*ssm.Options)) (*ssm.DeleteParametersOutput, error)
	GetParameters(ctx context.Context, params *ssm.GetParametersInput, optFns ...func(*ssm.Options)) (*ssm.GetParametersOutput, error)
	GetParametersByPath(ctx context.Context, params *ssm.GetParametersByPathInput, optFns ...func(*ssm.Options)) (*ssm.GetParametersByPathOutput, error)
}

var longRetryCfg = PanicOnError(config.LoadDefaultConfig(context.Background(),
	config.WithRetryer(func() aws.Retryer {
		// Try up to 5 times, reaching default max delay of 20s with exponential backk off on the 5th retry
		return retry.AddWithMaxAttempts(retry.NewStandard(), 5)
	},
	)))

var SsmClient SsmParametersAPI = ssm.NewFromConfig(longRetryCfg)

// TODO: this function is pretty useless, but it's here for consistency
func getSecretID(name string) *string {
	return ptr.String(name)
}

type ErrParameterNotFound = types.ParameterNotFound

// Deprecated: use ErrParameterNotFound directly
func IsParameterNotFoundError(err error) bool {
	var e *types.ParameterNotFound
	return errors.As(err, &e)
}

func (a *Aws) DeleteSecrets(ctx context.Context, names ...string) error {
	o, err := SsmClient.DeleteParameters(ctx, &ssm.DeleteParametersInput{
		Names: names, // works because getSecretID is a no-op
	})
	if err != nil {
		return err
	}
	if len(o.InvalidParameters) > 0 && len(o.DeletedParameters) == 0 {
		return &types.ParameterNotFound{}
	}
	return nil
}

func (a *Aws) IsValidSecret(ctx context.Context, name string) (bool, error) {
	secretId := getSecretID(name)
	res, err := SsmClient.DescribeParameters(ctx, &ssm.DescribeParametersInput{
		MaxResults: ptr.Int32(1),
		ParameterFilters: []types.ParameterStringFilter{
			{
				Key:    ptr.String("Name"),
				Option: ptr.String("Equals"),
				Values: []string{*secretId},
			},
		},
	})
	if err != nil {
		return false, err
	}
	return len(res.Parameters) == 1, nil
}

func (a *Aws) PutSecret(ctx context.Context, name, value string) error {
	secretId := getSecretID(name)
	secretString := ptr.String(value)

	// Call ssm:PutParameter
	_, err := SsmClient.PutParameter(ctx, &ssm.PutParameterInput{
		Overwrite: ptr.Bool(true),
		Type:      types.ParameterTypeSecureString,
		Name:      secretId,
		Value:     secretString,
		// SecretString: secretString,
	})
	if err != nil {
		return err
	}
	return nil
}

func (a *Aws) ListSecrets(ctx context.Context) ([]string, error) {
	return a.ListSecretsByPrefix(ctx, "")
}

func (a *Aws) ListSecretsByPrefix(ctx context.Context, prefix string) ([]string, error) {
	cfg, err := a.LoadConfig(ctx)
	if err != nil {
		return nil, err
	}

	svc := ssm.NewFromConfig(cfg)

	var filters []types.ParameterStringFilter
	// DescribeParameters fails if the BeginsWith value is empty
	if prefix := *getSecretID(prefix); prefix != "" {
		filters = append(filters, types.ParameterStringFilter{
			Key:    ptr.String("Name"),
			Option: ptr.String("BeginsWith"),
			Values: []string{prefix},
		})
	}

	var names []string
	var token *string
	for {
		res, err := svc.DescribeParameters(ctx, &ssm.DescribeParametersInput{
			// MaxResults: ptr.Int64(10); TODO: limit the output depending on quotas
			ParameterFilters: filters,
			NextToken:        token,
		})
		if err != nil {
			return nil, err
		}

		for _, p := range res.Parameters {
			names = append(names, *p.Name)
		}

		if token = res.NextToken; token == nil {
			break
		}
	}

	sort.Strings(names) // make sure the output is deterministic
	return names, nil
}
