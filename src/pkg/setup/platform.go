package setup

import (
	"fmt"

	"github.com/AlecAivazis/survey/v2"
	"github.com/DefangLabs/defang/src/pkg/surveyor"
	"github.com/DefangLabs/defang/src/pkg/term"
)

type SourcePlatform string

const (
	SourcePlatformHeroku SourcePlatform = "heroku"
)

func ParseSourcePlatform(input string) (SourcePlatform, error) {
	switch input {
	case string(SourcePlatformHeroku):
		return SourcePlatformHeroku, nil
	default:
		return "", fmt.Errorf("unknown source platform: %s", input)
	}
}

func selectSourcePlatform(surveyor surveyor.Surveyor) (error, SourcePlatform) {
	options := []string{
		string(SourcePlatformHeroku),
	}

	var selectedOption string

	for {
		err := surveyor.AskOne(&survey.Select{
			Message: "How is your project currently deployed?",
			Options: options,
			Help:    "Select the deployment platform you are currently using.",
		}, &selectedOption)
		if err != nil {
			return fmt.Errorf("failed to select source platform: %w", err), ""
		}

		sourcePlatform, err := ParseSourcePlatform(selectedOption)
		if err == nil {
			return nil, sourcePlatform
		}

		term.Warnf("Invalid source platform selected: %s. Please try again.", selectedOption)
	}
}
