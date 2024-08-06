package aws

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/aws/smithy-go/ptr"
)

const CONFIG_PATH_PART = "config"
const SENSITIVE_PATH_PART = "sensitive"

// TODO: this function is pretty useless, but it's here for consistency
func getConfigPathID(name string) *string {
	return ptr.String(name)
}

func getSensitiveConfigPathID(rootPath, name string) *string {
	return ptr.String(strings.Join([]string{rootPath, SENSITIVE_PATH_PART, name}, "/"))
}

func getNonSensitiveConfigPathID(rootPath, name string) *string {
	return ptr.String(strings.Join([]string{rootPath, CONFIG_PATH_PART, name}, "/"))
}

func IsParameterNotFoundError(err error) bool {
	var e *types.ParameterNotFound
	return errors.As(err, &e)
}

func (a *Aws) DeleteConfig(ctx context.Context, rootPath string, names ...string) error {
	cfg, err := a.LoadConfig(ctx)
	if err != nil {
		return err
	}

	svc := ssm.NewFromConfig(cfg)

	offset := len(names)
	paths := make([]string, 2*offset)

	for index, name := range names {
		paths[index] = *getSensitiveConfigPathID(rootPath, name)
		paths[offset+index] = *getNonSensitiveConfigPathID(rootPath, name)
	}

	o, err := svc.DeleteParameters(ctx, &ssm.DeleteParametersInput{
		Names: paths,
	})

	if err != nil {
		return fmt.Errorf("failed to delete configs: %v", err)
	}
	if len(o.InvalidParameters) > 0 && len(o.DeletedParameters) == 0 {
		return &types.ParameterNotFound{}
	}
	return nil
}

func (a *Aws) IsValidConfigName(ctx context.Context, name string) (bool, error) {
	cfg, err := a.LoadConfig(ctx)
	if err != nil {
		return false, err
	}

	rootPath := getConfigPathID("")
	svc := ssm.NewFromConfig(cfg)

	gpo, error := svc.GetParametersByPath(ctx,
		&ssm.GetParametersByPathInput{
			Path:           rootPath,
			Recursive:      aws.Bool(true),
			WithDecryption: aws.Bool(false),
		})

	if error != nil {
		return false, error
	}

	for _, param := range gpo.Parameters {
		parts := strings.Split(*param.Name, "/")
		if strings.EqualFold(parts[len(parts)-1], name) {
			return true, nil
		}
	}

	return false, nil
}

func errorOnDuplicateConfigExist(ctx context.Context, svc *ssm.Client, rootPath, name string, isSensitive bool) error {
	var altPath *string

	if isSensitive {
		altPath = getNonSensitiveConfigPathID(rootPath, name)
	} else {
		altPath = getSensitiveConfigPathID(rootPath, name)
	}

	_, err := svc.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           altPath,
		WithDecryption: ptr.Bool(false),
	})

	// param should not exist in any other path otherwise there is a conflict
	if err != nil {
		if !IsParameterNotFoundError(err) {
			return err
		}
	} else {
		// found in another path, return error
		if isSensitive {
			return errors.New("variable already exists as a non-sensitive")
		} else {
			return errors.New("variable already exists as a sensitive")
		}
	}

	return nil
}

func (a *Aws) PutConfig(ctx context.Context, rootPath, name, value string, isSensitive bool) error {
	cfg, err := a.LoadConfig(ctx)
	if err != nil {
		return err
	}

	configPath := getNonSensitiveConfigPathID(rootPath, name)
	if isSensitive {
		configPath = getSensitiveConfigPathID(rootPath, name)
	}
	configValue := ptr.String(value)
	svc := ssm.NewFromConfig(cfg)

	if err := errorOnDuplicateConfigExist(ctx, svc, rootPath, name, isSensitive); err != nil {
		return err
	}

	// Call ssm:PutParameter
	_, err = svc.PutParameter(ctx, &ssm.PutParameterInput{
		Overwrite: ptr.Bool(true),
		Type:      types.ParameterTypeSecureString,
		Name:      configPath,
		Value:     configValue,
	})

	if err != nil {
		return err
	}

	return nil
}

func GetConfigValuesByParam(ctx context.Context, svc *ssm.Client, rootPath string, names []string, isSensitive bool, outdata *defangv1.ConfigValues) error {
	namePaths := make([]string, len(names))

	var err error
	for index, name := range names {
		if isSensitive {
			namePaths[index] = *getSensitiveConfigPathID(rootPath, name)
		} else {
			namePaths[index] = *getNonSensitiveConfigPathID(rootPath, name)
		}
	}

	// 1. if sensitive ... tell user, don't need to have to fetch value
	// 2. get value
	gpo, err := svc.GetParameters(ctx, &ssm.GetParametersInput{
		WithDecryption: ptr.Bool(!isSensitive),
		Names:          namePaths,
	})

	if err != nil {
		if !IsParameterNotFoundError(err) {
			return err
		}
	}

	for _, param := range gpo.Parameters {
		(*outdata).Configs = append((*outdata).Configs,
			&defangv1.ConfigValue{
				Name:        *param.Name,
				Value:       *param.Value,
				IsSensitive: isSensitive,
			})
	}

	return nil
}

func (a *Aws) GetConfig(ctx context.Context, rootPath string, names ...string) (*defangv1.ConfigValues, error) {
	cfg, err := a.LoadConfig(ctx)
	if err != nil {
		return nil, err
	}

	svc := ssm.NewFromConfig(cfg)

	output := defangv1.ConfigValues{}
	if err := GetConfigValuesByParam(ctx, svc, rootPath, names, false, &output); err != nil {
		return nil, err
	}

	// we are done when output has the same number
	if len(output.Configs) == len(names) {
		return &output, nil
	}

	if err := GetConfigValuesByParam(ctx, svc, rootPath, names, true, &output); err != nil {
		return nil, err
	}

	return &output, nil
}

func (a *Aws) ListConfigs(ctx context.Context) ([]string, error) {
	return a.ListConfigsByPrefix(ctx, *getConfigPathID(""))
}

func (a *Aws) ListConfigsByPrefix(ctx context.Context, prefix string) ([]string, error) {
	cfg, err := a.LoadConfig(ctx)
	if err != nil {
		return nil, err
	}

	svc := ssm.NewFromConfig(cfg)

	var filters []types.ParameterStringFilter
	// DescribeParameters fails if the BeginsWith value is empty
	if prefix != "" {
		filters = append(filters, types.ParameterStringFilter{
			Key:    ptr.String("Name"),
			Option: ptr.String("BeginsWith"),
			Values: []string{prefix},
		})
	}

	res, err := svc.DescribeParameters(ctx, &ssm.DescribeParametersInput{
		// MaxResults: ptr.Int64(10); TODO: limit the output depending on quotas
		ParameterFilters: filters,
	})
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(res.Parameters))
	for _, p := range res.Parameters {
		names = append(names, *p.Name)
	}
	sort.Strings(names) // make sure the output is deterministic
	return names, nil
}
