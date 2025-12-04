package stacks

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/joho/godotenv"
)

type StackParameters struct {
	Name     string
	Provider client.ProviderID
	Region   string
	Mode     modes.Mode
}

var validStackName = regexp.MustCompile(`^[a-z][a-z0-9]*$`)

const dotDefang = ".defang"

func MakeDefaultName(providerId client.ProviderID, region string) string {
	compressedRegion := strings.ReplaceAll(region, "-", "")
	return strings.ToLower(providerId.String() + compressedRegion)
}

func Create(params StackParameters) (string, error) {
	if params.Name == "" {
		return "", errors.New("stack name cannot be empty")
	}
	if !validStackName.MatchString(params.Name) {
		return "", errors.New("stack name must start with a letter and contain only lowercase letters and numbers")
	}

	content, err := Marshal(params)
	if err != nil {
		return "", err
	}

	if err := os.Mkdir(dotDefang, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return "", err
	}
	filename := filename(params.Name)
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

type StackListItem struct {
	Name     string
	Provider string
	Region   string
	Mode     string
}

func List() ([]StackListItem, error) {
	files, err := os.ReadDir(dotDefang)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	var stacks []StackListItem
	for _, file := range files {
		filename := filepath.Join(dotDefang, file.Name())
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

func Marshal(params StackParameters) (string, error) {
	var properties map[string]string = make(map[string]string)
	properties["DEFANG_PROVIDER"] = strings.ToLower(params.Provider.String())
	if params.Region != "" {
		var regionVarName string
		switch params.Provider {
		case client.ProviderAWS:
			regionVarName = "AWS_REGION"
		case client.ProviderGCP:
			regionVarName = "GCP_LOCATION"
		}
		if regionVarName != "" {
			properties[regionVarName] = strings.ToLower(params.Region)
		}
	}
	if params.Mode != modes.ModeUnspecified {
		properties["DEFANG_MODE"] = strings.ToLower(params.Mode.String())
	}
	return godotenv.Marshal(properties)
}

func Remove(name string) error {
	if name == "" {
		return errors.New("stack name cannot be empty")
	}
	// delete the stack file
	return os.Remove(filename(name))
}

func filename(stackname string) string {
	return filepath.Join(dotDefang, stackname)
}
