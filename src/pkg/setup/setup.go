package setup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
)

func InteractiveSetup(ctx context.Context, client client.FabricClient, sourcePlatform SourcePlatform) error {
	term.Warn("Starting interactive setup")

	if sourcePlatform == "" {
		err, selected := selectSourcePlatform()
		if err != nil {
			return fmt.Errorf("failed to select source platform: %w", err)
		}
		sourcePlatform = selected
	}

	term.Debugf("Selected source platform: %s", sourcePlatform)

	switch sourcePlatform {
	case SourcePlatformHeroku:
		err := setupFromHeroku(ctx)
		if err != nil {
			return fmt.Errorf("failed to setup from Heroku: %w", err)
		}
	default:
		return fmt.Errorf("unsupported source platform: %s", sourcePlatform)
	}

	return nil
}

func setupFromHeroku(ctx context.Context) error {
	// invoke anthropic claude sonnet 4
	// Get API key from environment variable
	anthropicAPIKey := os.Getenv("ANTHROPIC_API_KEY")
	if anthropicAPIKey == "" {
		fmt.Println("Please set the ANTHROPIC_API_KEY environment variable")
		os.Exit(1)
	}

	claude := NewClaudeClient(anthropicAPIKey)

	token, err := getHerokuAuthToken()
	if err != nil {
		return fmt.Errorf("failed to get Heroku token: %w", err)
	}

	term.Debugf("Using Heroku token: %s", token)

	herokuClient := NewHerokuClient(token)
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

	sourceApp, err := selectSourceApplication(appNames)
	if err != nil {
		return fmt.Errorf("failed to select source application: %w", err)
	}

	term.Infof("Collecting information about %q...", sourceApp)

	applicationInfo, err := collectHerokuApplicationInfo(ctx, herokuClient, sourceApp)
	if err != nil {
		return fmt.Errorf("failed to collect Heroku application info: %w", err)
	}

	prompt := generateHerokuPrompt(applicationInfo)
	var composeFile string
	maxAttempts := 3
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		composeFile, err = generateComposeFile(claude, herokuSystemInstructions, prompt)
		if err == nil {
			break
		}
		term.Warnf("Failed to generate compose file from Heroku info (attempt %d/%d): %v", attempt, maxAttempts, err)
		if attempt < maxAttempts {
			term.Info("Retrying...")
			time.Sleep(2 * time.Second)
		}
	}
	if err != nil {
		return fmt.Errorf("failed to generate compose file from Heroku info after %d attempts: %w", maxAttempts, err)
	}

	term.Info(composeFile)

	return nil
}

func generateHerokuPrompt(info HerokuApplicationInfo) string {
	addonsJSON, err := json.Marshal(info.Addons)
	if err != nil {
		term.Warnf("Failed to marshal addons: %v", err)
		addonsJSON = []byte("[]")
	}
	dynosJSON, err := json.Marshal(info.Dynos)
	if err != nil {
		term.Warnf("Failed to marshal dynos: %v", err)
		dynosJSON = []byte("[]")
	}
	configVarsJSON, err := json.Marshal(info.ConfigVars)
	if err != nil {
		term.Warnf("Failed to marshal config vars: %v", err)
		configVarsJSON = []byte("{}")
	}

	return fmt.Sprintf(`Please provide a complete compose.yml file for the following application which is currently deployed to heroku. Explain any assumptions or recommendations inline using comments. Respond directly with the compose file contents in code fences.

## Application Details

Here are the Heroku Application Details

### Add-ons:

%s

### Dynos:

%s

### Config Vars:

%s
`, addonsJSON, dynosJSON, configVarsJSON)
}

func generateComposeFile(claude *ClaudeClient, systemInstructions, prompt string) (string, error) {
	claudeResponse, err := claude.SendConversation(systemInstructions, []Message{
		{
			Role:    "user",
			Content: prompt,
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to send message to Claude: %w", err)
	}

	term.Debugf("Claude response: %+v", claudeResponse)

	var responseText string

	for _, content := range claudeResponse.Content {
		if content.Type == "text" {
			responseText += content.Text
		} else {
			fmt.Printf("Unsupported content type: %s\n", content.Type)
		}
	}

	// parse the response text. if there are any code fences (```) in the response, extract the content inside the first code fence
	// avoid using regular expressions for simplicity
	if responseText == "" {
		return "", errors.New("Claude response is empty")
	}

	codeFenceStart := "```yaml"
	codeFenceEnd := "```"
	startIndex := strings.Index(responseText, codeFenceStart)
	if startIndex == -1 {
		return "", errors.New("no code fence found in Claude response")
	}
	startIndex += len(codeFenceStart)
	endIndex := strings.Index(responseText[startIndex:], codeFenceEnd)
	if endIndex == -1 {
		return "", errors.New("no closing code fence found in Claude response")
	}
	endIndex += startIndex

	responseText = responseText[startIndex:endIndex]

	return responseText, nil
}
