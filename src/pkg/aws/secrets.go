package aws

import (
	"context"
	"errors"
	"os"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/aws/smithy-go/ptr"
	"github.com/defang-io/defang/src/pkg"
)

var (
	currentUser = os.Getenv("USER") // TODO: sanitize
	stack       = pkg.Getenv("STACK", currentUser)
	stackPrefix = "/" + stack + "/"
)

func getSecretID(name string) *string {
	return ptr.String(stackPrefix + name)
}

func IsParameterNotFoundError(err error) bool {
	var e *types.ParameterNotFound
	return errors.As(err, &e)
}

func (a *Aws) DeleteSecret(ctx context.Context, name string) error {
	cfg, err := a.LoadConfig(ctx)
	if err != nil {
		return err
	}

	secretId := getSecretID(name)

	svc := ssm.NewFromConfig(cfg)

	_, err = svc.DeleteParameter(ctx, &ssm.DeleteParameterInput{
		Name: secretId,
	})
	return err
}

func (a *Aws) IsValidSecret(ctx context.Context, name string) (bool, error) {
	cfg, err := a.LoadConfig(ctx)
	if err != nil {
		return false, err
	}

	secretId := getSecretID(name)

	svc := ssm.NewFromConfig(cfg)

	res, err := svc.DescribeParameters(ctx, &ssm.DescribeParametersInput{
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
	cfg, err := a.LoadConfig(ctx)
	if err != nil {
		return err
	}

	secretId := getSecretID(name)
	secretString := ptr.String(value)

	svc := ssm.NewFromConfig(cfg)

	// Call ssm:PutParameter
	_, err = svc.PutParameter(ctx, &ssm.PutParameterInput{
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

	secretPrefix := getSecretID(prefix)

	svc := ssm.NewFromConfig(cfg)

	res, err := svc.DescribeParameters(ctx, &ssm.DescribeParametersInput{
		// MaxResults: ptr.Int64(10), TODO: limit the output depending on quotas
		ParameterFilters: []types.ParameterStringFilter{
			{
				Key:    ptr.String("Name"),
				Option: ptr.String("BeginsWith"),
				Values: []string{*secretPrefix},
			},
		},
	})
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(res.Parameters))
	for _, p := range res.Parameters {
		if name, found := strings.CutPrefix(*p.Name, stackPrefix); found {
			names = append(names, name)
		}
	}
	sort.Strings(names) // make sure the output is deterministic
	return names, nil
}
