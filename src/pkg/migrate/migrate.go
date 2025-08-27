package migrate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"slices"
	"strings"

	"al.essio.dev/pkg/shellescape"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/surveyor"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"go.yaml.in/yaml/v3"
)

func InteractiveSetup(ctx context.Context, fabric client.FabricClient, surveyor surveyor.Surveyor, heroku HerokuClientInterface, sourcePlatform SourcePlatform) (string, error) {
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
		_, err = compose.LoadFromContentWithInterpolation(ctx, []byte(composeContent), projectName)
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

func cleanupComposeFile(in string) (string, error) {
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(in), &document); err != nil {
		return "", fmt.Errorf("failed to unmarshal compose content as yaml: %w", err)
	}

	root := document.Content[0]
	node := root

	for i := 0; i < len(node.Content); i += 2 {
		ki := node.Content[i]
		vi := node.Content[i+1]
		if ki.Value == "version" {
			node.Content = slices.Delete(node.Content, i, i+2)
			i -= 2 // Adjust index after deletion
			continue
		}
		if ki.Value == "services" && vi.Kind == yaml.MappingNode {
			for j := 0; j < len(vi.Content); j += 2 {
				// kj := vi.Content[j]
				vj := vi.Content[j+1]
				for k := 0; k < len(vj.Content); k += 2 {
					kk := vj.Content[k]
					vk := vj.Content[k+1]
					if kk.Value == "image" && vk.Kind == yaml.ScalarNode {
						repo := compose.GetImageRepo(vk.Value)
						if compose.IsPostgresRepo(repo) {
							vj.Content = append(vj.Content, &yaml.Node{
								Tag:   "!!str",
								Kind:  yaml.ScalarNode,
								Value: "x-defang-postgres",
							}, &yaml.Node{
								Tag:   "!!str",
								Kind:  yaml.ScalarNode,
								Value: "true",
							})
						}
						if compose.IsMongoRepo(repo) {
							vj.Content = append(vj.Content, &yaml.Node{
								Tag:   "!!str",
								Kind:  yaml.ScalarNode,
								Value: "x-defang-mongodb",
							}, &yaml.Node{
								Tag:   "!!str",
								Kind:  yaml.ScalarNode,
								Value: "true",
							})
						}
						if compose.IsRedisRepo(repo) {
							vj.Content = append(vj.Content, &yaml.Node{
								Tag:   "!!str",
								Kind:  yaml.ScalarNode,
								Value: "x-defang-redis",
							}, &yaml.Node{
								Tag:   "!!str",
								Kind:  yaml.ScalarNode,
								Value: "true",
							})
						}
					}

					// railpack generates images with `Entrypoint: "bash -c"`, and
					// compose-go normalizes string commands into arrays, for example:
					// `command: npm start` -> `command: [ "npm", "start" ]`. As a
					// result, the command which ultimately gets run is
					// `bash -c npm start`. When this gets run, `bash` will ignore
					// `start` and `npm` will get run in a subprocess--only printing
					// the help text. As it is common for users to type their service
					// command as a string, this cleanup step will help ensure the
					// command is run as intended by replacing `command: npm start`
					// with `command: [ "npm start" ]`.
					if kk.Value == "command" {
						if vk.Kind == yaml.ScalarNode {
							cmd := vk.Value
							vk.Value = ""
							vk.Kind = yaml.SequenceNode
							vk.Tag = "!!seq"
							vk.Content = []*yaml.Node{
								{
									Tag:   "!!str",
									Kind:  yaml.ScalarNode,
									Value: cmd,
								},
							}
						}
						// We will do the same for
						// `command: ["npm", "start"]`, which will also get transformed
						// to `command: [ "npm start" ]`.
						if vk.Kind == yaml.SequenceNode && len(vk.Content) > 1 {
							var parts []string
							for _, c := range vk.Content {
								parts = append(parts, c.Value)
							}
							cmd := shellescape.QuoteCommand(parts)
							// combine content nodes into one
							vk.Content = []*yaml.Node{
								{
									Tag:   "!!str",
									Kind:  yaml.ScalarNode,
									Value: cmd,
								},
							}
						}
					}
				}
			}
		}
	}

	contentBytes, err := yaml.Marshal(root)
	if err != nil {
		return "", fmt.Errorf("failed to marshal compose content to yaml: %w", err)
	}

	return string(contentBytes), nil
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
