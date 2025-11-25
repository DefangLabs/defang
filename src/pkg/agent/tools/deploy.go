package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/agent/common"
	cliTypes "github.com/DefangLabs/defang/src/pkg/cli"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/logs"
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/DefangLabs/defang/src/pkg/stacks"
	"github.com/DefangLabs/defang/src/pkg/term"
)

var stack *string

func createNewStack(ctx context.Context, ec ElicitationsController) (*string, error) {
	response, err := ec.Request(ctx, ElicitationRequest{
		Message: "Where do you want to deploy?",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"provider": map[string]any{
					"type":        "string",
					"description": "Cloud Provider",
					"enum":        []string{"aws", "gcp", "digitalocean"},
				},
			},
			"required": []string{"provider"},
		},
	})

	if err != nil {
		return nil, fmt.Errorf("failed to elicit provider choice: %w", err)
	}

	providerName, ok := response.Content["provider"]
	if !ok {
		return nil, errors.New("invalid provider selection")
	}

	var providerID cliClient.ProviderID
	err = providerID.Set(providerName)
	if err != nil {
		return nil, err
	}
	defaultRegion := stacks.DefaultRegion(providerID)

	// create new stack
	response, err = ec.Request(ctx, ElicitationRequest{
		Message: "Which region do you want to deploy to?",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"region": map[string]any{
					"type":        "string",
					"description": "Cloud region",
					"default":     defaultRegion,
				},
			},
			"required": []string{"provider"},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to elicit region choice: %w", err)
	}

	region, ok := response.Content["region"]
	if !ok {
		return nil, errors.New("invalid region selection")
	}
	// TODO: ask for this
	name := fmt.Sprintf("%s-%s", strings.ToLower(providerID.String()), region)
	params := stacks.StackParameters{
		Provider: providerID,
		Region:   region,
		Name:     name,
	}
	_, err = stacks.Create(params)
	if err != nil {
		return nil, fmt.Errorf("failed to create stack: %w", err)
	}

	return &name, nil
}

func selectStack(ctx context.Context, ec ElicitationsController) (*string, error) {
	stackList, err := stacks.List()
	if err != nil {
		return nil, fmt.Errorf("failed to list stacks: %w", err)
	}

	if len(stackList) == 0 {
		return createNewStack(ctx, ec)
	}

	stackNames := make([]string, 0, len(stackList)+1)
	for _, s := range stackList {
		stackNames = append(stackNames, s.Name)
	}
	stackNames = append(stackNames, "Create new stack")

	// Prompt user to select or create stack
	response, err := ec.Request(ctx, ElicitationRequest{
		Message: "Select a stack",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"stack": map[string]any{
					"type":        "string",
					"description": "Which stack to use",
					"enum":        stackNames,
				},
			},
			"required": []string{"stack"},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to elicit stack choice: %w", err)
	}

	selectedStack, ok := response.Content["stack"]
	if !ok {
		return nil, errors.New("invalid stack selection")
	}
	return &selectedStack, nil
}

func setupStack(ctx context.Context, ec ElicitationsController) (*string, error) {
	if stack != nil {
		return stack, nil
	}

	selectedStack, err := selectStack(ctx, ec)
	if err != nil {
		return nil, fmt.Errorf("failed to select stack: %w", err)
	}
	if *selectedStack != "Create new stack" {
		return selectedStack, nil
	}

	return createNewStack(ctx, ec)
}

func HandleDeployTool(ctx context.Context, loader cliClient.ProjectLoader, providerId *cliClient.ProviderID, cluster string, cli CLIInterface, ec ElicitationsController) (string, error) {
	_, err := setupStack(ctx, ec)
	if err != nil {
		return "", fmt.Errorf("failed to setup stack: %w", err)
	}

	term.Debug("Function invoked: loader.LoadProject")
	project, err := cli.LoadProject(ctx, loader)
	if err != nil {
		err = fmt.Errorf("failed to parse compose file: %w", err)

		return "", fmt.Errorf("local deployment failed: %v. Please provide a valid compose file path.", err)
	}

	term.Debug("Function invoked: cli.Connect")
	client, err := cli.Connect(ctx, cluster)
	if err != nil {
		return "", fmt.Errorf("could not connect: %w", err)
	}

	term.Debug("Function invoked: cli.NewProvider")

	provider, err := cli.CheckProviderConfigured(ctx, client, *providerId, project.Name, "", len(project.Services))
	if err != nil {
		return "", fmt.Errorf("provider not configured correctly: %w", err)
	}

	// Deploy the services
	term.Debugf("Deploying services for project %s...", project.Name)

	term.Debug("Function invoked: cli.ComposeUp")
	// Use ComposeUp to deploy the services
	deployResp, project, err := cli.ComposeUp(ctx, client, provider, cliTypes.ComposeUpParams{
		Project:    project,
		UploadMode: compose.UploadModeDigest,
		Mode:       modes.ModeAffordable,
	})
	if err != nil {
		err = fmt.Errorf("failed to compose up services: %w", err)

		err = common.FixupConfigError(err)
		return "", err
	}

	if len(deployResp.Services) == 0 {
		return "", errors.New("no services deployed")
	}

	_, err = cli.TailAndMonitor(ctx, project, provider, 0, cliTypes.TailOptions{
		Follow:     true,
		Deployment: deployResp.Etag,
		Verbose:    true,
		LogType:    logs.LogTypeAll,
		Raw:        true,
	})
	if err != nil {
		return "", err
	}

	return "Deployment completed successfully", nil
}
