package tools

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/agent/common"
	"github.com/DefangLabs/defang/src/pkg/auth"
	cliTypes "github.com/DefangLabs/defang/src/pkg/cli"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/logs"
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/DefangLabs/defang/src/pkg/stacks"
	"github.com/DefangLabs/defang/src/pkg/term"
)

var stack *stacks.StackListItem

func ElicitString(ctx context.Context, ec ElicitationsController, message, field string) (string, error) {
	return elicitString(ctx, ec, message, field, "")
}

func ElicitStringWithDefault(ctx context.Context, ec ElicitationsController, message, field, defaultValue string) (string, error) {
	return elicitString(ctx, ec, message, field, defaultValue)
}

func elicitString(ctx context.Context, ec ElicitationsController, message, field, defaultValue string) (string, error) {
	response, err := ec.Request(ctx, ElicitationRequest{
		Message: message,
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				field: map[string]any{
					"type":        "string",
					"description": field,
					"default":     defaultValue,
				},
			},
			"required": []string{field},
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to elicit %s: %w", field, err)
	}

	value, ok := response.Content[field].(string)
	if !ok {
		return "", fmt.Errorf("invalid %s value", field)
	}

	return value, nil
}

func ElicitEnum(ctx context.Context, ec ElicitationsController, message, field string, options []string) (string, error) {
	response, err := ec.Request(ctx, ElicitationRequest{
		Message: message,
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				field: map[string]any{
					"type":        "string",
					"description": field,
					"enum":        options,
				},
			},
			"required": []string{field},
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to elicit %s: %w", field, err)
	}

	value, ok := response.Content[field].(string)
	if !ok {
		return "", fmt.Errorf("invalid %s value", field)
	}

	return value, nil
}

func createNewStack(ctx context.Context, ec ElicitationsController) (*stacks.StackListItem, error) {
	providerName, err := ElicitEnum(
		ctx,
		ec,
		"Where do you want to deploy?",
		"provider",
		[]string{"aws", "gcp", "digitalocean", "playground"},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to elicit provider choice: %w", err)
	}

	var providerID cliClient.ProviderID
	err = providerID.Set(providerName)
	if err != nil {
		return nil, err
	}
	// TODO: use cliClient.GetRegion(providerID)
	defaultRegion := stacks.DefaultRegion(providerID)
	region, err := ElicitStringWithDefault(ctx, ec, "Which region do you want to deploy to?", "region", defaultRegion)
	if err != nil {
		return nil, fmt.Errorf("failed to elicit region choice: %w", err)
	}

	// TODO: use the helper function (stacks.MakeDefaultName or something)
	defaultName := fmt.Sprintf("%s-%s", strings.ToLower(providerID.String()), region)
	name, err := ElicitStringWithDefault(ctx, ec, "Enter a name for your stack:", "stack_name", defaultName)
	if err != nil {
		return nil, fmt.Errorf("failed to elicit stack name: %w", err)
	}
	params := stacks.StackParameters{
		Provider: providerID,
		Region:   region,
		Name:     name,
	}
	_, err = stacks.Create(params)
	if err != nil {
		return nil, fmt.Errorf("failed to create stack: %w", err)
	}

	return &stacks.StackListItem{
		Name:     name,
		Provider: providerID.Name(),
		Region:   region,
	}, nil
}

func selectStack(ctx context.Context, ec ElicitationsController) (string, error) {
	stackList, err := stacks.List()
	if err != nil {
		return "", fmt.Errorf("failed to list stacks: %w", err)
	}

	if len(stackList) == 0 {
		return CreateNewStack, nil
	}

	stackNames := make([]string, 0, len(stackList)+1)
	for _, s := range stackList {
		stackNames = append(stackNames, s.Name)
	}
	stackNames = append(stackNames, CreateNewStack)

	selectedStackName, err := ElicitEnum(ctx, ec, "Select a stack", "stack", stackNames)
	if err != nil {
		return "", fmt.Errorf("failed to elicit stack choice: %w", err)
	}

	return selectedStackName, nil
}

const CreateNewStack = "Create new stack"

func setupStack(ctx context.Context, ec ElicitationsController) (*stacks.StackListItem, error) {
	if stack != nil {
		return stack, nil
	}

	selectedStackName, err := selectStack(ctx, ec)
	if err != nil {
		return nil, fmt.Errorf("failed to select stack: %w", err)
	}

	if selectedStackName == CreateNewStack {
		newStack, err := createNewStack(ctx, ec)
		if err != nil {
			return nil, fmt.Errorf("failed to create new stack: %w", err)
		}
		selectedStackName = newStack.Name
	}

	return stacks.Load(selectedStackName)
}

func SetupAWSAuthentication(ctx context.Context, ec ElicitationsController) error {
	// TODO: check the fs for AWS credentials file or config for profile names
	// TODO: add support for aws sso strategy
	strategy, err := ElicitEnum(ctx, ec, "How do you authenticate to AWS?", "strategy", []string{"access_key", "profile"})
	if err != nil {
		return fmt.Errorf("failed to elicit AWS Access Key ID: %w", err)
	}
	if strategy == "profile" {
		if os.Getenv("AWS_PROFILE") == "" {
			profile, err := ElicitString(ctx, ec, "Enter your AWS Profile Name:", "profile_name")
			if err != nil {
				return fmt.Errorf("failed to elicit AWS Profile Name: %w", err)
			}
			if err := os.Setenv("AWS_PROFILE", profile); err != nil {
				return fmt.Errorf("failed to set AWS_PROFILE environment variable: %w", err)
			}
		}
	} else {
		if os.Getenv("AWS_ACCESS_KEY_ID") == "" {
			accessKeyID, err := ElicitString(ctx, ec, "Enter your AWS Access Key ID:", "access_key_id")
			if err != nil {
				return fmt.Errorf("failed to elicit AWS Access Key ID: %w", err)
			}
			if err := os.Setenv("AWS_ACCESS_KEY_ID", accessKeyID); err != nil {
				return fmt.Errorf("failed to set AWS_ACCESS_KEY_ID environment variable: %w", err)
			}
		}
		if os.Getenv("AWS_SECRET_ACCESS_KEY") == "" {
			accessKeySecret, err := ElicitString(ctx, ec, "Enter your AWS Secret Access Key:", "access_key_secret")
			if err != nil {
				return fmt.Errorf("failed to elicit AWS Secret Access Key: %w", err)
			}
			if err := os.Setenv("AWS_SECRET_ACCESS_KEY", accessKeySecret); err != nil {
				return fmt.Errorf("failed to set AWS_SECRET_ACCESS_KEY environment variable: %w", err)
			}
		}
	}
	return nil
}

func SetupGCPAuthentication(ctx context.Context, ec ElicitationsController) error {
	if os.Getenv("GCP_PROJECT_ID") == "" {
		gcpProjectID, err := ElicitString(ctx, ec, "Enter your GCP Project ID:", "gcp_project_id")
		if err != nil {
			return fmt.Errorf("failed to elicit GCP Project ID: %w", err)
		}
		if err := os.Setenv("GCP_PROJECT_ID", gcpProjectID); err != nil {
			return fmt.Errorf("failed to set GCP_PROJECT_ID environment variable: %w", err)
		}
	}
	return nil
}

func SetupDOAuthentication(ctx context.Context, ec ElicitationsController) error {
	if os.Getenv("DIGITALOCEAN_TOKEN") == "" {
		pat, err := ElicitString(ctx, ec, "Enter your DigitalOcean Personal Access Token:", "personal_access_token")
		if err != nil {
			return fmt.Errorf("failed to elicit DigitalOcean Personal Access Token: %w", err)
		}
		if err := os.Setenv("DIGITALOCEAN_TOKEN", pat); err != nil {
			return fmt.Errorf("failed to set DIGITALOCEAN_TOKEN environment variable: %w", err)
		}
	}

	if os.Getenv("SPACES_ACCESS_KEY_ID") == "" {
		spaces_access_key, err := ElicitString(ctx, ec, "Enter your DigitalOcean Spaces Access Key:", "spaces_access_key")
		if err != nil {
			return fmt.Errorf("failed to elicit DigitalOcean Spaces Access Key: %w", err)
		}
		if err := os.Setenv("SPACES_ACCESS_KEY_ID", spaces_access_key); err != nil {
			return fmt.Errorf("failed to set SPACES_ACCESS_KEY_ID environment variable: %w", err)
		}
	}

	if os.Getenv("SPACES_SECRET_ACCESS_KEY") == "" {
		spaces_secret_key, err := ElicitString(ctx, ec, "Enter your DigitalOcean Spaces Secret Access Key:", "spaces_secret_access_key")
		if err != nil {
			return fmt.Errorf("failed to elicit DigitalOcean Spaces Secret Key: %w", err)
		}
		if err := os.Setenv("SPACES_SECRET_ACCESS_KEY", spaces_secret_key); err != nil {
			return fmt.Errorf("failed to set SPACES_SECRET_ACCESS_KEY environment variable: %w", err)
		}
	}
	return nil
}

func setupProviderAuthentication(ctx context.Context, ec ElicitationsController, providerId cliClient.ProviderID) error {
	switch providerId {
	case cliClient.ProviderAWS:
		return SetupAWSAuthentication(ctx, ec)
	case cliClient.ProviderGCP:
		return SetupGCPAuthentication(ctx, ec)
	case cliClient.ProviderDO:
		return SetupDOAuthentication(ctx, ec)
	}
	return nil
}

func HandleDeployTool(ctx context.Context, loader cliClient.ProjectLoader, providerId *cliClient.ProviderID, cluster string, cli CLIInterface, ec ElicitationsController) (string, error) {
	client, err := cli.Connect(ctx, cluster)
	if err != nil {
		term.Debug("Function invoked: cli.InteractiveLoginPrompt")
		err = cli.InteractiveLoginMCP(ctx, client, cluster, common.MCPDevelopmentClient)
		if err != nil {
			var noBrowserErr auth.ErrNoBrowser
			if errors.As(err, &noBrowserErr) {
				return noBrowserErr.Error(), nil
			}
			return "", err
		}
	}

	stack, err = setupStack(ctx, ec)
	if err != nil {
		return "", fmt.Errorf("failed to setup stack: %w", err)
	}

	providerId.Set(stack.Provider)

	err = setupProviderAuthentication(ctx, ec, *providerId)
	if err != nil {
		return "", fmt.Errorf("failed to setup provider authentication: %w", err)
	}

	term.Debug("Function invoked: loader.LoadProject")
	project, err := cli.LoadProject(ctx, loader)
	if err != nil {
		err = fmt.Errorf("failed to parse compose file: %w", err)

		return "", fmt.Errorf("local deployment failed: %v. Please provide a valid compose file path.", err)
	}

	provider, err := cli.CheckProviderConfigured(ctx, client, *providerId, project.Name, stack.Name, len(project.Services))
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
