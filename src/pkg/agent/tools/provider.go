package tools

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/elicitations"
	"github.com/DefangLabs/defang/src/pkg/stacks"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
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

func (pp *providerPreparer) SetupProvider(ctx context.Context, projectName string, stackName *string) (*cliClient.ProviderID, cliClient.Provider, error) {
	var providerID cliClient.ProviderID
	var err error
	var stack *stacks.StackParameters
	if stackName == nil {
		return nil, nil, errors.New("stackName cannot be nil")
	}
	if *stackName != "" {
		stack, err = loadStack(*stackName)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to load stack: %w", err)
		}
	} else {
		stack, err = pp.selectOrCreateStack(ctx, projectName)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to setup stack: %w", err)
		}
		*stackName = stack.Name
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
	provider := pp.pc.NewProvider(ctx, providerID, pp.fc, *stackName)
	return &providerID, provider, nil
}

type StackOption struct {
	Name  string
	Local bool
}

func (pp *providerPreparer) selectStack(ctx context.Context, ec elicitations.Controller, projectName string) (string, error) {
	remoteStackList, err := pp.collectExistingStacks(ctx, projectName)
	if err != nil {
		return "", fmt.Errorf("failed to collect existing stacks: %w", err)
	}
	localStackList, err := stacks.List()
	if err != nil {
		return "", fmt.Errorf("failed to list stacks: %w", err)
	}

	// Merge remote and local stacks into a single list of type StackOption,
	// prefer local if both exist
	stackMap := make(map[string]StackOption)
	for _, remoteStack := range remoteStackList {
		stackMap[remoteStack.Name] = StackOption{Name: remoteStack.Name, Local: false}
	}
	for _, localStack := range localStackList {
		stackMap[localStack.Name] = StackOption{Name: localStack.Name, Local: true}
	}
	if len(stackMap) == 0 {
		return CreateNewStack, nil
	}

	// Convert map back to slice
	stackNames := make([]string, 0, len(stackMap)+1)
	for _, stackOption := range stackMap {
		name := stackOption.Name
		if !stackOption.Local {
			name += " (remote)"
		}
		stackNames = append(stackNames, name)
	}
	stackNames = append(stackNames, CreateNewStack)

	selectedStackName, err := ec.RequestEnum(ctx, "Select a stack", "stack", stackNames)
	if err != nil {
		return "", fmt.Errorf("failed to elicit stack choice: %w", err)
	}

	// check if the selected stack is remote
	if strings.HasSuffix(selectedStackName, " (remote)") {
		selectedStackName = strings.TrimSuffix(selectedStackName, " (remote)")

		// find the stack parameters from the remoteStackList
		remoteStackParameters := &stacks.StackParameters{}
		found := false
		for _, remoteStack := range remoteStackList {
			if remoteStack.Name == selectedStackName {
				remoteStackParameters = remoteStack
				found = true
				break
			}
		}
		if !found {
			return "", fmt.Errorf("failed to find remote stack parameters for stack: %s", selectedStackName)
		}

		_, err := stacks.Create(*remoteStackParameters)
		if err != nil {
			return "", fmt.Errorf("failed to create local stack from remote: %w", err)
		}
	}

	return selectedStackName, nil
}

func (pp *providerPreparer) collectExistingStacks(ctx context.Context, projectName string) ([]*stacks.StackParameters, error) {
	resp, err := pp.fc.ListDeployments(ctx, &defangv1.ListDeploymentsRequest{
		Project: projectName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list deployments: %w", err)
	}
	deployments := resp.GetDeployments()
	stackMap := make(map[string]*stacks.StackParameters)
	for _, deployment := range deployments {
		stackName := deployment.GetStack()
		if stackName == "" {
			stackName = "beta"
		}
		var providerID cliClient.ProviderID
		providerID.SetValue(deployment.GetProvider())
		// overwrite existing entries to prefer the latest deployment
		stackMap[stackName] = &stacks.StackParameters{
			Name:     stackName,
			Provider: providerID,
			Region:   deployment.GetRegion(),
		}
	}
	stackParams := make([]*stacks.StackParameters, 0, len(stackMap))
	for _, params := range stackMap {
		stackParams = append(stackParams, params)
	}
	return stackParams, nil
}

func (pp *providerPreparer) selectOrCreateStack(ctx context.Context, projectName string) (*stacks.StackParameters, error) {
	selectedStackName, err := pp.selectStack(ctx, pp.ec, projectName)
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

	return loadStack(selectedStackName)
}

func loadStack(stackName string) (*stacks.StackParameters, error) {
	stack, err := stacks.Read(stackName)
	if err != nil {
		return nil, fmt.Errorf("failed to read stack: %w", err)
	}
	err = stacks.Load(stackName)
	if err != nil {
		return nil, fmt.Errorf("failed to load stack: %w", err)
	}
	return stack, nil
}

func (pp *providerPreparer) createNewStack(ctx context.Context) (*stacks.StackListItem, error) {
	var providerNames []string
	for _, p := range cliClient.AllProviders() {
		providerNames = append(providerNames, p.Name())
	}
	providerName, err := pp.ec.RequestEnum(
		ctx,
		"Where do you want to deploy?",
		"provider",
		providerNames,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to elicit provider choice: %w", err)
	}

	var providerID cliClient.ProviderID
	err = providerID.Set(providerName)
	if err != nil {
		return nil, err
	}

	var region string
	if providerID != cliClient.ProviderDefang { // no region for playground
		defaultRegion := cliClient.GetRegion(providerID)
		region, err = pp.ec.RequestStringWithDefault(ctx, "Which region do you want to deploy to?", "region", defaultRegion)
		if err != nil {
			return nil, fmt.Errorf("failed to elicit region choice: %w", err)
		}
	}

	defaultName := stacks.MakeDefaultName(providerID, region)
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
	if os.Getenv("AWS_PROFILE") != "" || (os.Getenv("AWS_ACCESS_KEY_ID") != "" && os.Getenv("AWS_SECRET_ACCESS_KEY") != "") {
		return nil
	}

	// TODO: check the fs for AWS credentials file or config for profile names
	// TODO: add support for aws sso strategy
	strategy, err := pp.ec.RequestEnum(ctx, "How do you authenticate to AWS?", "strategy", []string{
		"profile",
		"access_key",
	})
	if err != nil {
		return fmt.Errorf("failed to elicit AWS Access Key ID: %w", err)
	}
	if strategy == "profile" {
		if os.Getenv("AWS_PROFILE") == "" {
			knownProfiles, err := listAWSProfiles()
			if err != nil {
				return fmt.Errorf("failed to list AWS profiles: %w", err)
			}
			profile, err := pp.ec.RequestEnum(ctx, "Select your profile", "profile_name", knownProfiles)
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

func listAWSProfiles() ([]string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	files := []string{
		homeDir + "/.aws/credentials",
		homeDir + "/.aws/config",
	}

	profiles := make(map[string]struct{})

	for _, file := range files {
		f, err := os.Open(file)
		if err != nil {
			continue // skip missing files
		}

		var section string
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := scanner.Text()
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
				section = strings.Trim(line, "[]")
				// In config, profiles are named "profile NAME"
				section = strings.TrimPrefix(section, "profile ")
				profiles[section] = struct{}{}
			}
		}
		f.Close()
	}

	result := make([]string, 0, len(profiles))
	for p := range profiles {
		result = append(result, p)
	}
	sort.Strings(result)
	return result, nil
}
