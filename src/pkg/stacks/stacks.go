package stacks

import (
	"errors"
	"os"
	"regexp"
	"strings"

	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/joho/godotenv"
)

type StackParameters struct {
	Name     string
	Provider cliClient.ProviderID
	Region   string
	Mode     modes.Mode
}

var validStackName = regexp.MustCompile(`^[a-z][a-z0-9]*$`)

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

	filename := filename(params.Name)
	file, err := os.CreateTemp(".", filename+".tmp.")
	if err != nil {
		return "", err
	}
	defer file.Close()

	_, err = file.WriteString(content)
	if err != nil {
		return "", err
	}

	term.Debugf("Created tmp stack configuration file: %s\n", file.Name())

	// move to final name
	err = os.Rename(file.Name(), filename)
	if err != nil {
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
	files, err := os.ReadDir(".")
	if err != nil {
		return nil, err
	}

	var stacks []StackListItem
	for _, file := range files {
		if strings.HasPrefix(file.Name(), ".defangrc.") {
			content, err := os.ReadFile(file.Name())
			if err != nil {
				term.Warnf("Skipping unreadable stack file %s: %v\n", file.Name(), err)
				continue
			}
			params, err := Parse(string(content))
			if err != nil {
				term.Warnf("Skipping invalid stack file %s: %v\n", file.Name(), err)
				continue
			}
			params.Name = strings.TrimPrefix(file.Name(), ".defangrc.")

			stacks = append(stacks, StackListItem{
				Name:     params.Name,
				Provider: params.Provider.String(),
				Region:   params.Region,
				Mode:     params.Mode.String(),
			})
		}
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
			params.Provider = cliClient.ProviderID(value)
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
		case cliClient.ProviderAWS:
			regionVarName = "AWS_REGION"
		case cliClient.ProviderGCP:
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
	// delete the stack rc file
	return os.Remove(filename(name))
}

func filename(stackname string) string {
	return ".defangrc." + stackname
}
