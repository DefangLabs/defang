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
	Name         string
	Provider     client.ProviderID
	Region       string
	AWSProfile   string
	GCPProjectID string
	Mode         modes.Mode
}

func (params StackParameters) ToMap() map[string]string {
	var properties map[string]string = make(map[string]string)
	properties["DEFANG_PROVIDER"] = strings.ToLower(params.Provider.String())
	if params.Region != "" {
		regionVarName := client.GetRegionVarName(params.Provider)
		properties[regionVarName] = strings.ToLower(params.Region)
	}
	if params.Mode != modes.ModeUnspecified {
		properties["DEFANG_MODE"] = strings.ToLower(params.Mode.String())
	}

	if params.Provider == client.ProviderAWS && params.AWSProfile != "" {
		properties["AWS_PROFILE"] = params.AWSProfile
	}
	if params.Provider == client.ProviderGCP && params.GCPProjectID != "" {
		properties["GCP_PROJECT_ID"] = params.GCPProjectID
	}
	return properties
}

func ParamsFromMap(properties map[string]string) (StackParameters, error) {
	var params StackParameters
	for key, value := range properties {
		switch key {
		case "DEFANG_PROVIDER":
			if err := params.Provider.Set(value); err != nil {
				return params, err
			}
		case "AWS_REGION":
			params.Region = value
		case "GCP_LOCATION":
			params.Region = value
		case "AWS_PROFILE":
			params.AWSProfile = value
		case "GCP_PROJECT_ID":
			params.GCPProjectID = value
		case "DEFANG_MODE":
			mode, err := modes.Parse(value)
			if err != nil {
				return params, err
			}
			params.Mode = mode
		}
	}
	return params, nil
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

func Create(params StackParameters) (string, error) {
	return CreateInDirectory(".", params)
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
	Name         string
	AWSProfile   string
	GCPProjectID string
	Provider     string
	Region       string
	Mode         string
	DeployedAt   time.Time
}

func (sli StackListItem) ToParameters() StackParameters {
	var providerID client.ProviderID
	providerID.Set(sli.Provider)
	mode, err := modes.Parse(sli.Mode)
	if err != nil {
		mode = modes.ModeUnspecified
	}
	return StackParameters{
		Name:         sli.Name,
		Provider:     providerID,
		Region:       sli.Region,
		AWSProfile:   sli.AWSProfile,
		GCPProjectID: sli.GCPProjectID,
		Mode:         mode,
	}
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
		params, err := Parse(string(content))
		if err != nil {
			term.Warnf("Skipping invalid stack file %s: %v\n", filename, err)
			continue
		}
		params.Name = file.Name()

		stacks = append(stacks, StackListItem{
			Name:     params.Name,
			Provider: params.Provider.String(),
			Region:   params.Region,
			Mode:     params.Mode.String(),
		})
	}

	return stacks, nil
}

func Parse(content string) (StackParameters, error) {
	properties, err := godotenv.Parse(strings.NewReader(content))
	if err != nil {
		return StackParameters{}, err
	}

	return ParamsFromMap(properties)
}

func Marshal(params *StackParameters) (string, error) {
	return godotenv.Marshal(params.ToMap())
}

func Remove(name string) error {
	return RemoveInDirectory(".", name)
}

func RemoveInDirectory(workingDirectory, name string) error {
	if name == "" {
		return errors.New("stack name cannot be empty")
	}
	path := filename(workingDirectory, name)
	// delete the stack file
	return os.Remove(path)
}

func Read(name string) (*StackParameters, error) {
	return ReadInDirectory(".", name)
}

func ReadInDirectory(workingDirectory, name string) (*StackParameters, error) {
	path := filename(workingDirectory, name)
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("could not read stack %q from %q: %w", name, path, err)
	}
	parsed, err := Parse(string(content))
	if err != nil {
		return nil, err
	}
	parsed.Name = name
	return &parsed, nil
}

func Load(name string) error {
	return LoadInDirectory(".", name)
}

func LoadInDirectory(workingDirectory, name string) error {
	path := filename(workingDirectory, name)
	if err := godotenv.Load(path); err != nil {
		return fmt.Errorf("could not load stack %q from %q %w", name, path, err)
	}

	term.Debugf("loaded globals from %s", path)
	return nil
}

func Overload(name string) error {
	return OverloadInDirectory(".", name)
}

func OverloadInDirectory(workingDirectory, name string) error {
	path := filename(workingDirectory, name)
	if err := godotenv.Overload(path); err != nil {
		return fmt.Errorf("could not load stack %q from %q %w", name, path, err)
	}

	term.Debugf("loaded globals from %s", path)
	return nil
}

// This was basically ripped out of godotenv.Overload/Load. Unfortunately, they don't export
// a function that loads a map[string]string, so we have to reimplement it here.
func LoadParameters(params map[string]string, overload bool) error {
	currentEnv := map[string]bool{}
	rawEnv := os.Environ()
	for _, rawEnvLine := range rawEnv {
		key := strings.Split(rawEnvLine, "=")[0]
		currentEnv[key] = true
	}

	for key, value := range params {
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
