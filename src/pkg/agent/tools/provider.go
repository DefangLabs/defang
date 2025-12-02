package tools

import (
	"context"
	"fmt"
	"os"
	"strings"

	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/elicitations"
	"github.com/DefangLabs/defang/src/pkg/stacks"
	"github.com/DefangLabs/defang/src/pkg/term"
)

const CreateNewStack = "Create new stack"

type ProviderCreator interface {
	NewProvider(ctx context.Context, providerId cliClient.ProviderID, client cliClient.FabricClient, stack string) cliClient.Provider
}

type providerPreparer struct {
	pc ProviderCreator
	ec elicitations.Controller
	fc cliClient.FabricClient
}

func NewProviderPreparer(pc ProviderCreator, ec elicitations.Controller, fc cliClient.FabricClient) *providerPreparer {
	return &providerPreparer{
		pc: pc,
		ec: ec,
		fc: fc,
	}
}

// TODO: call this in each tool which requires a provider
func (pp *providerPreparer) SetupProvider(ctx context.Context, stackName string) (*cliClient.ProviderID, cliClient.Provider, error) {
	var providerID cliClient.ProviderID
	var err error
	var stack *stacks.StackParameters
	if stackName != "" {
		stack, err = stacks.Read(stackName)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to read stack: %w", err)
		}
		err = stacks.Load(stackName)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to load stack: %w", err)
		}
	} else {
		stack, err = pp.setupStack(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to setup stack: %w", err)
		}
	}

	err = providerID.Set(stack.Provider.Name())
	if err != nil {
		return nil, nil, fmt.Errorf("failed to set provider ID: %w", err)
	}

	err = pp.setupProviderAuthentication(ctx, providerID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to setup provider authentication: %w", err)
	}

	term.Debug("Function invoked: cli.NewProvider")
	provider := pp.pc.NewProvider(ctx, providerID, pp.fc, stackName)
	return &providerID, provider, nil
}

func selectStack(ctx context.Context, ec elicitations.Controller) (string, error) {
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

	selectedStackName, err := ec.RequestEnum(ctx, "Select a stack", "stack", stackNames)
	if err != nil {
		return "", fmt.Errorf("failed to elicit stack choice: %w", err)
	}

	return selectedStackName, nil
}

func (pp *providerPreparer) setupStack(ctx context.Context) (*stacks.StackParameters, error) {
	selectedStackName, err := selectStack(ctx, pp.ec)
	if err != nil {
		return nil, fmt.Errorf("failed to select stack: %w", err)
	}

	if selectedStackName == CreateNewStack {
		newStack, err := pp.createNewStack(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to create new stack: %w", err)
		}
		selectedStackName = newStack.Name
	}

	err = stacks.Load(selectedStackName)
	if err != nil {
		return nil, fmt.Errorf("failed to load stack: %w", err)
	}

	return stacks.Read(selectedStackName)
}

func (pp *providerPreparer) createNewStack(ctx context.Context) (*stacks.StackListItem, error) {
	providerName, err := pp.ec.RequestEnum(
		ctx,
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
	defaultRegion := cliClient.GetRegion(providerID)
	region, err := pp.ec.RequestStringWithDefault(ctx, "Which region do you want to deploy to?", "region", defaultRegion)
	if err != nil {
		return nil, fmt.Errorf("failed to elicit region choice: %w", err)
	}

	// TODO: use the helper function (stacks.MakeDefaultName or something)
	defaultName := fmt.Sprintf("%s-%s", strings.ToLower(providerID.String()), region)
	name, err := pp.ec.RequestStringWithDefault(ctx, "Enter a name for your stack:", "stack_name", defaultName)
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

func (pp *providerPreparer) setupProviderAuthentication(ctx context.Context, providerId cliClient.ProviderID) error {
	switch providerId {
	case cliClient.ProviderAWS:
		return pp.SetupAWSAuthentication(ctx)
	case cliClient.ProviderGCP:
		return pp.SetupGCPAuthentication(ctx)
	case cliClient.ProviderDO:
		return pp.SetupDOAuthentication(ctx)
	}
	return nil
}

func (pp *providerPreparer) SetupAWSAuthentication(ctx context.Context) error {
	// TODO: check the fs for AWS credentials file or config for profile names
	// TODO: add support for aws sso strategy
	strategy, err := pp.ec.RequestEnum(ctx, "How do you authenticate to AWS?", "strategy", []string{"access_key", "profile"})
	if err != nil {
		return fmt.Errorf("failed to elicit AWS Access Key ID: %w", err)
	}
	if strategy == "profile" {
		if os.Getenv("AWS_PROFILE") == "" {
			profile, err := pp.ec.RequestString(ctx, "Enter your AWS Profile Name:", "profile_name")
			if err != nil {
				return fmt.Errorf("failed to elicit AWS Profile Name: %w", err)
			}
			if err := os.Setenv("AWS_PROFILE", profile); err != nil {
				return fmt.Errorf("failed to set AWS_PROFILE environment variable: %w", err)
			}
		}
	} else {
		if os.Getenv("AWS_ACCESS_KEY_ID") == "" {
			accessKeyID, err := pp.ec.RequestString(ctx, "Enter your AWS Access Key ID:", "access_key_id")
			if err != nil {
				return fmt.Errorf("failed to elicit AWS Access Key ID: %w", err)
			}
			if err := os.Setenv("AWS_ACCESS_KEY_ID", accessKeyID); err != nil {
				return fmt.Errorf("failed to set AWS_ACCESS_KEY_ID environment variable: %w", err)
			}
		}
		if os.Getenv("AWS_SECRET_ACCESS_KEY") == "" {
			accessKeySecret, err := pp.ec.RequestString(ctx, "Enter your AWS Secret Access Key:", "access_key_secret")
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

func (pp *providerPreparer) SetupGCPAuthentication(ctx context.Context) error {
	if os.Getenv("GCP_PROJECT_ID") == "" {
		gcpProjectID, err := pp.ec.RequestString(ctx, "Enter your GCP Project ID:", "gcp_project_id")
		if err != nil {
			return fmt.Errorf("failed to elicit GCP Project ID: %w", err)
		}
		if err := os.Setenv("GCP_PROJECT_ID", gcpProjectID); err != nil {
			return fmt.Errorf("failed to set GCP_PROJECT_ID environment variable: %w", err)
		}
	}
	return nil
}

func (pp *providerPreparer) SetupDOAuthentication(ctx context.Context) error {
	if os.Getenv("DIGITALOCEAN_TOKEN") == "" {
		pat, err := pp.ec.RequestString(ctx, "Enter your DigitalOcean Personal Access Token:", "personal_access_token")
		if err != nil {
			return fmt.Errorf("failed to elicit DigitalOcean Personal Access Token: %w", err)
		}
		if err := os.Setenv("DIGITALOCEAN_TOKEN", pat); err != nil {
			return fmt.Errorf("failed to set DIGITALOCEAN_TOKEN environment variable: %w", err)
		}
	}

	if os.Getenv("SPACES_ACCESS_KEY_ID") == "" {
		spaces_access_key, err := pp.ec.RequestString(ctx, "Enter your DigitalOcean Spaces Access Key:", "spaces_access_key")
		if err != nil {
			return fmt.Errorf("failed to elicit DigitalOcean Spaces Access Key: %w", err)
		}
		if err := os.Setenv("SPACES_ACCESS_KEY_ID", spaces_access_key); err != nil {
			return fmt.Errorf("failed to set SPACES_ACCESS_KEY_ID environment variable: %w", err)
		}
	}

	if os.Getenv("SPACES_SECRET_ACCESS_KEY") == "" {
		spaces_secret_key, err := pp.ec.RequestString(ctx, "Enter your DigitalOcean Spaces Secret Access Key:", "spaces_secret_access_key")
		if err != nil {
			return fmt.Errorf("failed to elicit DigitalOcean Spaces Secret Key: %w", err)
		}
		if err := os.Setenv("SPACES_SECRET_ACCESS_KEY", spaces_secret_key); err != nil {
			return fmt.Errorf("failed to set SPACES_SECRET_ACCESS_KEY environment variable: %w", err)
		}
	}
	return nil
}
