package setup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/surveyor"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"gopkg.in/yaml.v3"
)

func InteractiveSetup(ctx context.Context, fabric client.FabricClient, surveyor surveyor.Surveyor, heroku HerokuClientInterface, sourcePlatform SourcePlatform) (string, error) {
	term.Warn("Starting interactive setup")

	if sourcePlatform == "" {
		err, selected := selectSourcePlatform(surveyor)
		if err != nil {
			return "", fmt.Errorf("failed to select source platform: %w", err)
		}
		sourcePlatform = selected
	}

	term.Debugf("Selected source platform: %s", sourcePlatform)

	var composeFileContents string
	var err error
	switch sourcePlatform {
	case SourcePlatformHeroku:
		composeFileContents, err = setupFromHeroku(ctx, fabric, surveyor, heroku)
		if err != nil {
			return "", fmt.Errorf("failed to setup from Heroku: %w", err)
		}
	default:
		return "", fmt.Errorf("unsupported source platform: %s", sourcePlatform)
	}

	return composeFileContents, nil
}

func setupFromHeroku(ctx context.Context, fabric client.FabricClient, surveyor surveyor.Surveyor, herokuClient HerokuClientInterface) (string, error) {
	token, err := getHerokuAuthToken()
	if err != nil {
		return "", fmt.Errorf("failed to get Heroku auth token: %w", err)
	}
	herokuClient.SetToken(token)
	apps, err := herokuClient.ListApps(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to list Heroku apps: %w", err)
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
		return "", fmt.Errorf("failed to select source application: %w", err)
	}

	term.Infof("Collecting information about %q...", sourceApp)

	applicationInfo, err := collectHerokuApplicationInfo(ctx, herokuClient, sourceApp)
	if err != nil {
		return "", fmt.Errorf("failed to collect Heroku application info: %w", err)
	}

	term.Debugf("Application info: %+v\n", applicationInfo)

	sanitizedApplicationInfo, err := sanitizeHerokuApplicationInfo(applicationInfo)
	if err != nil {
		return "", fmt.Errorf("failed to sanitize Heroku application info: %w", err)
	}

	term.Debugf("Sanitized application info: %+v\n", sanitizedApplicationInfo)

	term.Info("Generating compose file...")

	composeFileContents, err := generateComposeFile(ctx, fabric, defangv1.SourcePlatform_SOURCE_PLATFORM_HEROKU, sourceApp, sanitizedApplicationInfo)
	if err != nil {
		return "", fmt.Errorf("failed to generate compose file from Heroku info: %w", err)
	}

	return composeFileContents, nil
}

func sanitizeHerokuApplicationInfo(info HerokuApplicationInfo) (interface{}, error) {
	for key, value := range info.ConfigVars {
		// Redact sensitive information in config vars
		isSecret, err := compose.IsSecret(value)
		if err != nil {
			return nil, fmt.Errorf("failed to check if config var %q is a secret: %w", key, err)
		}
		if isSecret {
			info.ConfigVars[key] = "REDACTED"
		}
	}

	return info, nil
}

func generateComposeFile(ctx context.Context, fabric client.FabricClient, platform defangv1.SourcePlatform, projectName string, data any) (string, error) {
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

		responseStr := string(resp.Compose)
		term.Debugf("Received compose response: %+v", responseStr)

		// assume the response is markdown,
		// extract the contents of the first code block if there is one
		composeContent, err := extractFirstCodeBlock(responseStr)
		if err != nil {
			if errors.Is(err, errNoCodeBlock) {
				// If no code block found, use the entire response
				composeContent = responseStr
			} else {
				previousError = err.Error()
				term.Debugf("Failed to extract code block: %v. Retrying...", err)
				continue
			}
		}

		// Cleanup the compose content
		composeContent, err = cleanupComposeFile(composeContent)
		if err != nil {
			previousError = err.Error()
			continue
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

func cleanupComposeFile(composeContent string) (string, error) {
	// parse as yaml and remove `version` property if present
	var composeYAML map[string]interface{}
	if err := yaml.Unmarshal([]byte(composeContent), &composeYAML); err != nil {
		return "", fmt.Errorf("failed to unmarshal compose content as yaml: %w", err)
	}
	delete(composeYAML, "version")

	composeBytes, err := yaml.Marshal(composeYAML)
	if err != nil {
		return "", fmt.Errorf("failed to marshal compose content to yaml: %w", err)
	}

	return string(composeBytes), nil
}

func newline() string {
	if runtime.GOOS == "windows" {
		return "\r\n"
	}
	return "\n"
}

var errNoCodeBlock = errors.New("no code block found")

// extractFirstCodeBlock extracts the first code block from markdown text
// It looks for fenced code blocks (```...```) and returns the content inside
func extractFirstCodeBlock(markdown string) (string, error) {
	newline := newline()
	lines := strings.Split(markdown, newline)
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

	if start == -1 || end == -1 {
		return "", errNoCodeBlock
	}

	if end <= start+1 {
		return "", errors.New("empty code block found")
	}

	codeBlock := strings.Join(lines[start+1:end], newline)
	return codeBlock, nil
}
