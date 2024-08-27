package aws

import (
	"context"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"

	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/aws/smithy-go/ptr"
)

const CONFIG_PATH_PART = "config"
const SENSITIVE_PATH_PART = ""

// TODO: this function is pretty useless, but it's here for consistency
func getConfigPathID(name string) *string {
	return ptr.String(name)
}

func stripPath(name string) string {
	lastIndex := strings.LastIndex(name, "/")
	return name[lastIndex+1:]
}

func getSensitiveConfigPathID(rootPath, name string) *string {
	root := strings.TrimRight(strings.Trim(rootPath, " "), "/")
	path.Join()
	return ptr.String(path.Join(root, SENSITIVE_PATH_PART, name))
}

func getNonSensitiveConfigPathID(rootPath, name string) *string {
	root := strings.TrimRight(strings.Trim(rootPath, " "), "/")
	return ptr.String(path.Join(root, CONFIG_PATH_PART, name))
}

func IsParameterNotFoundError(err error) bool {
	var e *types.ParameterNotFound
	return errors.As(err, &e)
}

func (a *Aws) DeleteConfigs(ctx context.Context, rootPath string, names ...string) error {
	cfg, err := a.LoadConfig(ctx)
	if err != nil {
		return err
	}

	svc := ssm.NewFromConfig(cfg)

	offset := len(names)
	paths := make([]string, 2*offset)

	// since we don't know whether the config is sensitive or not,
	// we delete from both paths expecting half to be invalid
	for index, name := range names {
		paths[index] = *getSensitiveConfigPathID(rootPath, name)
		paths[offset+index] = *getNonSensitiveConfigPathID(rootPath, name)
	}

	dpo, err := svc.DeleteParameters(ctx, &ssm.DeleteParametersInput{
		Names: paths,
	})
	if err != nil {
		return fmt.Errorf("failed to delete all configs: %v", err)
	}

	if len(dpo.DeletedParameters) < len(names) {
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
		return false, fmt.Errorf("failed to validate config name: %s", name)
	}

	for _, param := range gpo.Parameters {
		parts := strings.Split(*param.Name, "/")
		if strings.EqualFold(parts[len(parts)-1], name) {
			return true, nil
		}
	}

	return false, nil
}

func doesConfigNameExist(ctx context.Context, svc *ssm.Client, rootPath, name string, isSensitive bool) bool {
	var path *string
	if isSensitive {
		path = getSensitiveConfigPathID(rootPath, name)
	} else {
		path = getNonSensitiveConfigPathID(rootPath, name)
	}

	_, err := svc.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           path,
		WithDecryption: ptr.Bool(false),
	})

	// param should not exist in any other path otherwise there is a conflict
	if err != nil {
		if IsParameterNotFoundError(err) {
			return false
		}
	}

	return true
}

func (a *Aws) PutConfig(ctx context.Context, rootPath, name, value string, isSensitive bool) error {
	cfg, err := a.LoadConfig(ctx)
	if err != nil {
		return err
	}

	svc := ssm.NewFromConfig(cfg)
	if doesConfigNameExist(ctx, svc, rootPath, name, !isSensitive) {
		if isSensitive {
			return fmt.Errorf("cannot change %q from non-sensitive to sensitive", name)
		} else {
			return fmt.Errorf("cannot change %q from sensitive to non-sensitive", name)
		}
	}

	var configPath *string
	if isSensitive {
		configPath = getSensitiveConfigPathID(rootPath, name)
	} else {
		configPath = getNonSensitiveConfigPathID(rootPath, name)
	}

	_, err = svc.PutParameter(ctx, &ssm.PutParameterInput{
		Overwrite: ptr.Bool(true),
		Type:      types.ParameterTypeSecureString,
		Name:      configPath,
		Value:     ptr.String(value),
	})

	if err != nil {
		return errors.New("failed to save config")
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
		return errors.New("failed to get config")
	}

	for _, param := range gpo.Parameters {
		value := ""
		if !isSensitive {
			value = *param.Value
		}

		(*outdata).Configs = append((*outdata).Configs,
			&defangv1.ConfigValue{
				Name:        *param.Name,
				Value:       value,
				IsSensitive: isSensitive,
			})
	}

	return nil
}

func (a *Aws) GetConfigs(ctx context.Context, rootPath string, names ...string) (*defangv1.ConfigValues, error) {
	// if no names are provided, get all the configs
	if len(names) == 0 {
		list, err := a.ListConfigsByPrefix(ctx, rootPath)
		if err != nil {
			return nil, err
		}

		for _, name := range list {
			lastIndex := strings.LastIndex(name, "/")
			names = append(names, name[lastIndex+1:])
		}
	}

	cfg, err := a.LoadConfig(ctx)
	if err != nil {
		return nil, err
	}

	svc := ssm.NewFromConfig(cfg)

	output := defangv1.ConfigValues{}
	if err := GetConfigValuesByParam(ctx, svc, rootPath, names, false, &output); err != nil {
		return nil, err
	}

	// if we didn't get all the configs, try to get the rest as sensitives
	if len(output.Configs) != len(names) {
		if err := GetConfigValuesByParam(ctx, svc, rootPath, names, true, &output); err != nil {
			return nil, err
		}
	}

	for _, config := range output.Configs {
		config.Name = stripPath(config.Name)
	}
	return &output, nil
}

func (a *Aws) ListConfigs(ctx context.Context, projectName string) ([]string, error) {
	return a.ListConfigsByPrefix(ctx, *getConfigPathID(""))
}

func (a *Aws) ListConfigsByPrefix(ctx context.Context, prefix string) ([]string, error) {
	cfg, err := a.LoadConfig(ctx)
	if err != nil {
		return nil, err
	}

	svc := ssm.NewFromConfig(cfg)

	var nextToken *string = nil
	var names = make([]string, 0)
	for {
		res, err := svc.GetParametersByPath(ctx, &ssm.GetParametersByPathInput{
			Path:           &prefix,
			Recursive:      aws.Bool(true),
			NextToken:      nextToken,
			WithDecryption: aws.Bool(false),
		})
		if err != nil {
			return nil, errors.New("failed to get of list configs")
		}

		for _, p := range res.Parameters {
			name := stripPath(*p.Name)
			names = append(names, name)
		}
		if res.NextToken == nil {
			break
		}
		nextToken = res.NextToken
	}
	sort.Strings(names)
	return names, nil
}
