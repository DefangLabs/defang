package stacks

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/DefangLabs/defang/src/pkg/term"
)

type StackParameters struct {
	Name     string
	Provider cliClient.ProviderID
	Region   string
	Mode     modes.Mode
}

func Create(params StackParameters) error {
	if params.Name == "" {
		return errors.New("stack name cannot be empty")
	}
	if !regexp.MustCompile(`^[a-zA-Z0-9]+$`).MatchString(params.Name) {
		return errors.New("stack name must be alphanumeric")
	}

	content := Marshal(params)
	filename := params.Name + ".defangrc"
	file, err := os.CreateTemp(".", filename+".tmp.")
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.WriteString(content)
	if err != nil {
		return err
	}

	term.Debugf("Created tmp stack configuration file: %s\n", file.Name())

	// move to final name
	err = os.Rename(file.Name(), filename)
	if err != nil {
		return err
	}

	term.Infof(
		"Created new stack configuration file: `%s`. "+
			"Check this file into version control. "+
			"You can now deploy this stack using `defang up %s`\n",
		filename, params.Name,
	)

	return nil
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
		if strings.HasSuffix(file.Name(), ".defangrc") {
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
			params.Name = strings.TrimSuffix(file.Name(), ".defangrc")

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
	var params StackParameters
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "DEFANG_PROVIDER=") {
			providerStr := strings.TrimPrefix(line, "DEFANG_PROVIDER=")
			params.Provider = cliClient.ProviderID(providerStr)
		} else if strings.HasPrefix(line, "AWS_REGION=") {
			params.Region = strings.TrimPrefix(line, "AWS_REGION=")
		} else if strings.HasPrefix(line, "GCP_REGION=") {
			params.Region = strings.TrimPrefix(line, "GCP_REGION=")
		} else if strings.HasPrefix(line, "DEFANG_MODE=") {
			modeStr := strings.TrimPrefix(line, "DEFANG_MODE=")
			mode, err := modes.Parse(modeStr)
			if err != nil {
				return params, err
			}
			params.Mode = mode
		}
	}
	return params, nil
}

func Marshal(params StackParameters) string {
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("DEFANG_PROVIDER=%s\n", strings.ToLower(params.Provider.String())))
	if params.Region != "" {
		var regionVarName string
		switch params.Provider {
		case cliClient.ProviderAWS:
			regionVarName = "AWS_REGION"
		case cliClient.ProviderGCP:
			regionVarName = "GCP_REGION"
		}
		if regionVarName != "" {
			builder.WriteString(fmt.Sprintf("%s=%s\n", regionVarName, strings.ToLower(params.Region)))
		}
	}
	if params.Mode != modes.ModeUnspecified {
		builder.WriteString(fmt.Sprintf("DEFANG_MODE=%s\n", strings.ToLower(params.Mode.String())))
	}
	return builder.String()
}

func Remove(name string) error {
	if name == "" {
		return errors.New("stack name cannot be empty")
	}
	// delete the stack rc file
	filename := name + ".defangrc"
	return os.Remove(filename)
}
