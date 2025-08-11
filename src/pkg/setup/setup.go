package setup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type Surveyor interface {
	AskOne(prompt survey.Prompt, response interface{}, opts ...survey.AskOpt) error
}

type DefaultSurveyor struct {
	DefaultOpts []survey.AskOpt
}

func NewDefaultSurveyor() *DefaultSurveyor {
	return &DefaultSurveyor{
		DefaultOpts: []survey.AskOpt{survey.WithStdio(term.DefaultTerm.Stdio())},
	}
}

func (ds *DefaultSurveyor) AskOne(prompt survey.Prompt, response interface{}, opts ...survey.AskOpt) error {
	return survey.AskOne(prompt, response, append(ds.DefaultOpts, opts...)...)
}

func InteractiveSetup(ctx context.Context, fabric client.FabricClient, surveyor Surveyor, heroku HerokuClientInterface, sourcePlatform SourcePlatform) error {
	term.Warn("Starting interactive setup")

	if sourcePlatform == "" {
		err, selected := selectSourcePlatform(surveyor)
		if err != nil {
			return fmt.Errorf("failed to select source platform: %w", err)
		}
		sourcePlatform = selected
	}

	term.Debugf("Selected source platform: %s", sourcePlatform)

	switch sourcePlatform {
	case SourcePlatformHeroku:
		err := setupFromHeroku(ctx, fabric, surveyor, heroku)
		if err != nil {
			return fmt.Errorf("failed to setup from Heroku: %w", err)
		}
	default:
		return fmt.Errorf("unsupported source platform: %s", sourcePlatform)
	}

	return nil
}

func setupFromHeroku(ctx context.Context, fabric client.FabricClient, surveyor Surveyor, herokuClient HerokuClientInterface) error {
	token, err := getHerokuAuthToken()
	if err != nil {
		return fmt.Errorf("failed to get Heroku auth token: %w", err)
	}
	herokuClient.SetToken(token)
	apps, err := herokuClient.ListApps(ctx)
	if err != nil {
		return fmt.Errorf("failed to list Heroku apps: %w", err)
	}

	// Here you can add logic to process the retrieved apps and set up the project accordingly
	// For now, we just print the apps
	term.Debugf("Your Heroku applications: %+v\n", apps)

	appNames := make([]string, len(apps))
	for i, app := range apps {
		appNames[i] = app.Name
	}

	sourceApp, err := selectSourceApplication(surveyor, appNames)
	if err != nil {
		return fmt.Errorf("failed to select source application: %w", err)
	}

	term.Infof("Collecting information about %q...", sourceApp)

	applicationInfo, err := collectHerokuApplicationInfo(ctx, herokuClient, sourceApp)
	if err != nil {
		return fmt.Errorf("failed to collect Heroku application info: %w", err)
	}

	term.Info("Generating compose file...")

	composeFile, err := generateComposeFile(ctx, fabric, defangv1.SourcePlatform_HEROKU, sourceApp, sanitizedApplicationInfo)
	if err != nil {
		return fmt.Errorf("failed to generate compose file from Heroku info: %w", err)
	}

	term.Info(composeFile)

	return nil
}

func generateComposeFile(ctx context.Context, fabric client.FabricClient, platform defangv1.SourcePlatform, data interface{}) (string, error) {
	var err error
	dataJSON, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("failed to marshal data to json: %w", err)
	}

	var resp *defangv1.GenerateComposeResponse
	var previousError string
	for range [3]int{} {
		resp, err = fabric.GenerateCompose(ctx, &defangv1.GenerateComposeRequest{
			Platform:      platform,
			Data:          dataJSON,
			PreviousError: previousError,
		})
		if err != nil {
			return "", err
		}

		responseStr := string(resp.GetCompose())
		term.Debugf("Received compose response: %+v", responseStr)

		// assume the response is markdown,
		// extract the contents of the first code block
		composeContent := extractFirstCodeBlock(responseStr)
		if composeContent == "" {
			// If no code block found, use the entire response
			composeContent = responseStr
		}

		// Attempt to load the compose content
		_, err = compose.LoadFromContent(ctx, []byte(composeContent), projectName)
		if err != nil {
			previousError = err.Error()
			term.Debugf("Invalid compose file received: %v. Retrying...", err)
			continue
		}

		// If we reach here, the compose content is valid
		return composeContent, nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to generate compose file after retries: %w", err)
	}

	// This should not be reached, but just in case
	return "", errors.New("unexpected error: no valid compose file generated")
}

// extractFirstCodeBlock extracts the first code block from markdown text
// It looks for fenced code blocks (```...```) and returns the content inside
func extractFirstCodeBlock(markdown string) string {
	lines := strings.Split(markdown, "\n")
	start := -1
	end := -1

	for i, line := range lines {
		if strings.HasPrefix(line, "```") {
			if start == -1 {
				start = i
			} else {
				end = i
				break
			}
		}
	}

	if start != -1 && end != -1 && end > start+1 {
		return strings.TrimSpace(strings.Join(lines[start+1:end], "\n"))
	}

	return ""
}
