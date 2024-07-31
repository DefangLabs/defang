package aws

import (
	"context"
	"errors"
	"sort"
	"strings"

	clitypes "github.com/DefangLabs/defang/src/pkg/types"
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

func insertPreConfigNamePath(name, pathPart string) (*string, error) {
	// add the new path part to the name (we will swap positions of the
	// variable name part with the new path part below)
	pathParts := strings.Split(name+"/"+pathPart, "/")
	if len(pathParts) < 2 {
		return nil, errors.New("invalid config name")
	}

	// swap the variable name part with the new path part
	varPart := pathParts[len(pathParts)-2]
	pathParts[len(pathParts)-2] = pathPart
	pathParts[len(pathParts)-1] = varPart

	// rejoin everthing and return
	return ptr.String(strings.Join(pathParts, "/")), nil
}

func IsParameterNotFoundError(err error) bool {
	var e *types.ParameterNotFound
	return errors.As(err, &e)
}

func (a *Aws) DeleteSecrets(ctx context.Context, names ...string) error {
	cfg, err := a.LoadConfig(ctx)
	if err != nil {
		return err
	}

	svc := ssm.NewFromConfig(cfg)

	o, err := svc.DeleteParameters(ctx, &ssm.DeleteParametersInput{
		Names: names, // works because getConfigPathID is a no-op
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

func errorOnDuplicateConfigExist(ctx context.Context, svc *ssm.Client, name string, isSensitive bool) error {
	var altPath *string

	var err error
	if isSensitive {
		altPath, err = insertPreConfigNamePath(name, SENSITIVE_PATH_PART)
	} else {
		altPath, err = insertPreConfigNamePath(name, CONFIG_PATH_PART)
	}

	if err != nil {
		return err
	}

	_, err = svc.GetParameter(ctx, &ssm.GetParameterInput{
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

func (a *Aws) PutConfig(ctx context.Context, name, value string, isSensitive bool) error {
	cfg, err := a.LoadConfig(ctx)
	if err != nil {
		return err
	}

	configPath := getConfigPathID(name)
	configValue := ptr.String(value)

	svc := ssm.NewFromConfig(cfg)

	if err := errorOnDuplicateConfigExist(ctx, svc, name, isSensitive); err != nil {
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

func GetConfigValuesByParam(ctx context.Context, svc *ssm.Client, names []string, isSensitive bool, outdata *clitypes.ConfigData) error {
	namePaths := make([]string, len(names))

	insertPathPart := CONFIG_PATH_PART
	if isSensitive {
		insertPathPart = SENSITIVE_PATH_PART
	}

	var err error
	var basePath string
	var path *string
	for index, name := range names {
		basePath = *getConfigPathID(name)

		path, err = insertPreConfigNamePath(basePath, insertPathPart)
		if err != nil {
			return err
		}
		namePaths[index] = *path
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
		(*outdata)[*param.Name] = clitypes.DataInfo{
			Value:       *param.Value,
			IsSensitive: isSensitive,
		}
	}

	return nil
}

func (a *Aws) GetConfig(ctx context.Context, names []string) (clitypes.ConfigData, error) {
	cfg, err := a.LoadConfig(ctx)
	if err != nil {
		return nil, err
	}

	svc := ssm.NewFromConfig(cfg)

	output := clitypes.ConfigData{}
	if err := GetConfigValuesByParam(ctx, svc, names, false, &output); err != nil {
		return nil, err
	}

	// we are done when output has the same number
	if len(output) == len(names) {
		return output, nil
	}

	if err := GetConfigValuesByParam(ctx, svc, names, true, &output); err != nil {
		return nil, err
	}

	return output, nil
}

func (a *Aws) ListConfigs(ctx context.Context) ([]string, error) {
	return a.ListConfigsByPrefix(ctx, "")
}

func (a *Aws) ListConfigsByPrefix(ctx context.Context, prefix string) ([]string, error) {
	cfg, err := a.LoadConfig(ctx)
	if err != nil {
		return nil, err
	}

	svc := ssm.NewFromConfig(cfg)

	var filters []types.ParameterStringFilter
	// DescribeParameters fails if the BeginsWith value is empty
	if prefix := *getConfigPathID(prefix); prefix != "" {
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
