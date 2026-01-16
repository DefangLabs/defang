package stacks

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/joho/godotenv"
)

type StackParameters struct {
	Name     string
	Provider client.ProviderID
	Mode     modes.Mode
	Region   string
	// replace properties with variable map, but keep getters and setters for common ones
	Variables map[string]string
}

func (sp StackParameters) ToMap() map[string]string {
	// make a copy to avoid modifying the original
	vars := make(map[string]string, len(sp.Variables))
	for k, v := range sp.Variables {
		vars[k] = v
	}
	vars["DEFANG_PROVIDER"] = sp.Provider.String()
	regionVarName := client.GetRegionVarName(sp.Provider)
	if regionVarName != "" && sp.Region != "" {
		vars[regionVarName] = sp.Region
	}
	if sp.Mode != modes.ModeUnspecified {
		vars["DEFANG_MODE"] = strings.ToLower(sp.Mode.String())
	}
	return vars
}

func ParamsFromMap(variables map[string]string) (StackParameters, error) {
	if variables == nil {
		return StackParameters{}, errors.New("properties map cannot be nil")
	}
	var provider client.ProviderID
	if val, ok := variables["DEFANG_PROVIDER"]; ok {
		err := provider.Set(val)
		if err != nil {
			return StackParameters{}, fmt.Errorf("invalid DEFANG_PROVIDER value %q: %w", val, err)
		}
	}
	var mode modes.Mode
	if val, ok := variables["DEFANG_MODE"]; ok {
		err := mode.Set(val)
		if err != nil {
			return StackParameters{}, fmt.Errorf("invalid DEFANG_MODE value %q: %w", val, err)
		}
	}
	regionVarName := client.GetRegionVarName(provider)
	region := variables[regionVarName]
	return StackParameters{
		Variables: variables,
		Provider:  provider,
		Region:    region,
		Mode:      mode,
	}, nil
}

var validStackName = regexp.MustCompile(`^[a-z][a-z0-9]*$`)

const (
	DefaultBeta = "beta"
	Directory   = ".defang"
)

func MakeDefaultName(providerId client.ProviderID, region string) string {
	compressedRegion := strings.ReplaceAll(region, "-", "")
	return strings.ToLower(providerId.String() + compressedRegion)
}

func CreateInDirectory(workingDirectory string, params StackParameters) (string, error) {
	if params.Name == "" {
		return "", errors.New("stack name cannot be empty")
	}
	if !validStackName.MatchString(params.Name) {
		return "", errors.New("stack name must start with a letter and contain only lowercase letters and numbers")
	}

	content, err := Marshal(&params)
	if err != nil {
		return "", err
	}

	defangDir := filepath.Join(workingDirectory, Directory)
	if err := os.MkdirAll(defangDir, 0700); err != nil {
		return "", err
	}
	filename := filename(workingDirectory, params.Name)
	f, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			instructions := fmt.Sprintf(
				"If you want to overwrite it, please spin down the stack and remove stackfile first.\n"+
					"    defang down --stack %s && rm .defang/%s",
				params.Name,
				params.Name,
			)
			return "", fmt.Errorf(
				"stack file already exists for %q.\n%s",
				params.Name,
				instructions,
			)
		}
		return "", err
	}
	defer f.Close()

	_, err = f.WriteString(content)
	if err != nil {
		os.Remove(filename)
		return "", err
	}

	return filename, nil
}

// for shell printing for converting to string format of StackParameters
type StackListItem struct {
	StackParameters
	DeployedAt time.Time
}

func List() ([]StackListItem, error) {
	return ListInDirectory(".")
}

func ListInDirectory(workingDirectory string) ([]StackListItem, error) {
	defangDir := filepath.Join(workingDirectory, Directory)
	files, err := os.ReadDir(defangDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	var stacks []StackListItem
	for _, file := range files {
		filename := filename(workingDirectory, file.Name())
		content, err := os.ReadFile(filename)
		if err != nil {
			term.Warnf("Skipping unreadable stack file %s: %v\n", filename, err)
			continue
		}
		variables, err := Parse(string(content))
		if err != nil {
			term.Warnf("Skipping invalid stack file %s: %v\n", filename, err)
			continue
		}
		params, err := ParamsFromMap(variables)
		if err != nil {
			term.Warnf("Skipping invalid stack file %s: %v\n", filename, err)
			continue
		}
		params.Name = file.Name()
		stacks = append(stacks, StackListItem{
			StackParameters: params,
		})
	}

	return stacks, nil
}

func Parse(content string) (map[string]string, error) {
	return godotenv.Parse(strings.NewReader(content))
}

func Marshal(params *StackParameters) (string, error) {
	if params == nil {
		return "", nil
	}
	return godotenv.Marshal(params.ToMap())
}

func RemoveInDirectory(workingDirectory, name string) error {
	if name == "" {
		return errors.New("stack name cannot be empty")
	}
	path := filename(workingDirectory, name)
	// delete the stack file
	return os.Remove(path)
}

func ReadInDirectory(workingDirectory, name string) (*StackParameters, error) {
	path := filename(workingDirectory, name)
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	variables, err := Parse(string(content))
	if err != nil {
		return nil, err
	}
	params, err := ParamsFromMap(variables)
	if err != nil {
		return nil, fmt.Errorf("could not parse stack %q: %w", name, err)
	}
	params.Name = name
	return &params, nil
}

// This was basically ripped out of godotenv.Overload/Load. Unfortunately, they don't export
// a function that loads a map[string]string, so we have to reimplement it here.
func LoadStackEnv(params StackParameters, overload bool) error {
	currentEnv := map[string]bool{}
	rawEnv := os.Environ()
	for _, rawEnvLine := range rawEnv {
		key := strings.Split(rawEnvLine, "=")[0]
		currentEnv[key] = true
	}

	paramsMap := params.ToMap()
	for key, value := range paramsMap {
		if currentEnv[key] && !overload {
			term.Warnf("The environment variable %q is set in both the stackfile and the environment. The value from the environment will be used.\n", key)
		}
		if !currentEnv[key] || overload {
			err := os.Setenv(key, value)
			if err != nil {
				return fmt.Errorf("could not set env var %q: %w", key, err)
			}
		}
	}

	return nil
}

func filename(workingDirectory, stackname string) string {
	return filepath.Join(workingDirectory, Directory, stackname)
}

func PostCreateMessage(stackName string) string {
	return fmt.Sprintf(
		"A stackfile has been created at `.defang/%s`.\n"+
			"This file contains the configuration for this stack.\n"+
			"We recommend you commit this file to source control, so it can be used by everyone on your team.\n"+
			"You can now deploy using `defang up --stack=%s`.\n"+
			"To learn more about stacks, visit https://docs.defang.io/docs/concepts/stacks",
		stackName, stackName,
	)
}
