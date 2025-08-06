package setup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func InteractiveSetup(ctx context.Context, fabric client.FabricClient, sourcePlatform SourcePlatform) error {
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
		err := setupFromHeroku(ctx, fabric)
		if err != nil {
			return fmt.Errorf("failed to setup from Heroku: %w", err)
		}
	default:
		return fmt.Errorf("unsupported source platform: %s", sourcePlatform)
	}

	return nil
}

func setupFromHeroku(ctx context.Context, fabric client.FabricClient) error {
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

	composeFile, err := generateComposeFile(ctx, fabric, defangv1.SourcePlatform_HEROKU, applicationInfo)
	if err != nil {
		return errors.New("failed to generate compose file from Heroku info")
	}

	term.Info(composeFile)

	return nil
}

func generateComposeFile(ctx context.Context, fabric client.FabricClient, platform defangv1.SourcePlatform, data interface{}) (string, error) {
	dataJSON, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("failed to marshal data to json: %w", err)
	}

	resp, err := fabric.GenerateCompose(ctx, &defangv1.GenerateComposeRequest{
		Platform: platform,
		Data:     dataJSON,
	})
	if err != nil {
		return "", fmt.Errorf("failed to call GenerateCompose: %w", err)
	}

	return string(resp.GetCompose()), nil
}
