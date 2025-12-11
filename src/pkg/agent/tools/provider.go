package tools

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

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

type StacksManager interface {
	Create(params stacks.StackParameters) (string, error)
	Read(stackName string) (*stacks.StackParameters, error)
	LoadParameters(*stacks.StackParameters)
	List() ([]stacks.StackListItem, error)
}

type stacksManager struct{}

func NewStacksManager() *stacksManager {
	return &stacksManager{}
}

func (sm *stacksManager) Create(params stacks.StackParameters) (string, error) {
	return stacks.Create(params)
}

func (sm *stacksManager) Read(stackName string) (*stacks.StackParameters, error) {
	return stacks.Read(stackName)
}

func (sm *stacksManager) LoadParameters(params *stacks.StackParameters) {
	stacks.LoadParameters(params)
}

func (sm *stacksManager) List() ([]stacks.StackListItem, error) {
	return stacks.List()
}

type providerPreparer struct {
	pc ProviderCreator
	ec elicitations.Controller
	fc cliClient.FabricClient
	sm StacksManager
}

func NewProviderPreparer(pc ProviderCreator, ec elicitations.Controller, fc cliClient.FabricClient) *providerPreparer {
	return &providerPreparer{
		pc: pc,
		ec: ec,
		fc: fc,
		sm: NewStacksManager(),
	}
}

func (pp *providerPreparer) SetupProvider(ctx context.Context, projectName string, stackName *string, useWkDir bool) (*cliClient.ProviderID, cliClient.Provider, error) {
	var providerID cliClient.ProviderID
	var err error
	var stack *stacks.StackParameters
	if stackName == nil {
		return nil, nil, errors.New("stackName cannot be nil")
	}
	if *stackName == "" {
		stack, err = pp.selectOrCreateStack(ctx, projectName, useWkDir)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to setup stack: %w", err)
		}
		*stackName = stack.Name
	} else {
		stack, err = pp.getStackParameters(ctx, projectName, *stackName, useWkDir)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to load stack: %w", err)
		}
	}

	term.Debugf("Loading stack params %v", stack)
	pp.sm.LoadParameters(stack)
	err = providerID.Set(stack.Provider.Name())
	if err != nil {
		return nil, nil, fmt.Errorf("failed to set provider ID: %w", err)
	}

	if useWkDir {
		_, err = pp.sm.Create(*stack)
		if err != nil {
			term.Warnf("Failed to create stackfile: %v", err)
		}
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
	Name           string
	Local          bool
	LastDeployedAt time.Time
	Parameters     *stacks.StackParameters
}

func (pp *providerPreparer) collectStackOptions(ctx context.Context, projectName string, useWkDir bool) (map[string]StackOption, error) {
	// Merge remote and local stacks into a single list of type StackOption,
	// prefer local if both exist
	stackMap := make(map[string]StackOption)
	remoteStackList, err := pp.collectPreviouslyDeployedStacks(ctx, projectName)
	if err != nil {
		return nil, fmt.Errorf("failed to collect existing stacks: %w", err)
	}
	for _, remoteStack := range remoteStackList {
		stackMap[remoteStack.Name] = StackOption{
			Name:           remoteStack.Name,
			Local:          false,
			LastDeployedAt: remoteStack.DeployedAt,
			Parameters:     &remoteStack.StackParameters,
		}
	}

	if useWkDir {
		localStackList, err := pp.sm.List()
		if err != nil {
			return nil, fmt.Errorf("failed to list stacks: %w", err)
		}

		for _, localStack := range localStackList {
			existing, exists := stackMap[localStack.Name]
			lastDeployedAt := time.Time{}
			if exists {
				lastDeployedAt = existing.LastDeployedAt
			}
			stackMap[localStack.Name] = StackOption{
				Name:           localStack.Name,
				Local:          true,
				LastDeployedAt: lastDeployedAt,
				Parameters:     nil,
			}
		}
	}

	stackLabelMap := make(map[string]StackOption)
	for _, stackOption := range stackMap {
		label := stackOption.Name
		if !stackOption.LastDeployedAt.IsZero() {
			label = fmt.Sprintf("%s (last deployed %s)", stackOption.Name, stackOption.LastDeployedAt.Local().Format(time.RFC822))
		}
		stackLabelMap[label] = stackOption
	}

	return stackLabelMap, nil
}

func printStacksInfoMessage(stacks map[string]StackOption) {
	_, betaExists := stacks["beta"]
	if betaExists {
		infoLine := "This project was deployed with an implicit Stack called 'beta' before Stacks were introduced."
		if len(stacks) == 1 {
			infoLine += "\n   To update your existing deployment, select the 'beta' Stack.\n" +
				"Creating a new Stack will result in a separate deployment instance."
		}
		infoLine += "\n   To learn more about Stacks, visit: https://docs.defang.io/docs/concepts/stacks"
		term.Info(infoLine + "\n")
	}
	executable, _ := os.Executable()
	term.Infof("To skip this prompt, run %s up --stack=%s", filepath.Base(executable), "<stack_name>")
}

func (pp *providerPreparer) selectStack(ctx context.Context, projectName string, useWkDir bool) (*StackOption, error) {
	stackOptions, err := pp.collectStackOptions(ctx, projectName, useWkDir)
	if err != nil {
		return nil, fmt.Errorf("failed to collect stack options: %w", err)
	}
	if len(stackOptions) == 0 {
		return &StackOption{Name: CreateNewStack}, nil
	}

	printStacksInfoMessage(stackOptions)

	// Convert map back to slice
	stackLabels := make([]string, 0, len(stackOptions)+1)
	for label := range stackOptions {
		stackLabels = append(stackLabels, label)
	}
	if useWkDir {
		stackLabels = append(stackLabels, CreateNewStack)
	}

	selectedStackLabel, err := pp.ec.RequestEnum(ctx, "Select a stack", "stack", stackLabels)
	if err != nil {
		return nil, fmt.Errorf("failed to elicit stack choice: %w", err)
	}

	// Handle special case where user selects "Create new stack"
	if selectedStackLabel == CreateNewStack {
		return &StackOption{Name: CreateNewStack}, nil
	}

	selectedStackOption, ok := stackOptions[selectedStackLabel]
	if !ok {
		return nil, fmt.Errorf("selected stack label %q not found in stack options map", selectedStackLabel)
	}
	if selectedStackOption.Local {
		return &selectedStackOption, nil
	}

	if selectedStackOption.Parameters == nil {
		return nil, fmt.Errorf("stack parameters for remote stack %q are nil", selectedStackLabel)
	}

	if useWkDir {
		term.Debugf("Importing stack %s from remote", selectedStackLabel)
		_, err = pp.sm.Create(*selectedStackOption.Parameters)
		if err != nil {
			return nil, fmt.Errorf("failed to create local stack from remote: %w", err)
		}
	}

	return &selectedStackOption, nil
}

type ExistingStack struct {
	stacks.StackParameters
	DeployedAt time.Time
}

func (pp *providerPreparer) collectPreviouslyDeployedStacks(ctx context.Context, projectName string) ([]*ExistingStack, error) {
	resp, err := pp.fc.ListDeployments(ctx, &defangv1.ListDeploymentsRequest{
		Project: projectName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list deployments: %w", err)
	}
	deployments := resp.GetDeployments()
	stackMap := make(map[string]*ExistingStack)
	for _, deployment := range deployments {
		stackName := deployment.GetStack()
		if stackName == "" {
			stackName = "beta"
		}
		var providerID cliClient.ProviderID
		providerID.SetValue(deployment.GetProvider())
		// avoid overwriting existing entries, deployments are already sorted by deployed_at desc
		if _, exists := stackMap[stackName]; !exists {
			var deployedAt time.Time
			if ts := deployment.GetTimestamp(); ts != nil {
				deployedAt = ts.AsTime()
			}
			stackMap[stackName] = &ExistingStack{
				StackParameters: stacks.StackParameters{
					Name:     stackName,
					Provider: providerID,
					Region:   deployment.GetRegion(),
				},
				DeployedAt: deployedAt,
			}
		}
	}
	stackParams := make([]*ExistingStack, 0, len(stackMap))
	for _, params := range stackMap {
		stackParams = append(stackParams, params)
	}
	return stackParams, nil
}

func (pp *providerPreparer) selectOrCreateStack(ctx context.Context, projectName string, useWkDir bool) (*stacks.StackParameters, error) {
	selectedStack, err := pp.selectStack(ctx, projectName, useWkDir)
	if err != nil {
		return nil, fmt.Errorf("failed to select stack: %w", err)
	}

	if selectedStack.Name == CreateNewStack {
		newStack, err := pp.promptForStackParameters(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to create new stack: %w", err)
		}
		return newStack, nil
	}

	// For local stacks, parameters need to be loaded from the stack manager
	if selectedStack.Local && selectedStack.Parameters == nil {
		params, err := pp.sm.Read(selectedStack.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to read local stack %q: %w", selectedStack.Name, err)
		}
		return params, nil
	}

	return selectedStack.Parameters, nil
}

func (pp *providerPreparer) getStackParameters(ctx context.Context, projectName, stackName string, useWkDir bool) (*stacks.StackParameters, error) {
	if !useWkDir {
		return pp.importRemoteStack(ctx, projectName, stackName)
	}

	stack, err := pp.sm.Read(stackName)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("failed to read stack: %w", err)
		}
		stack, err = pp.importRemoteStack(ctx, projectName, stackName)
		if err != nil {
			return nil, fmt.Errorf("failed to import remote stack: %w", err)
		}
		if stack == nil {
			return nil, fmt.Errorf("stack %q does not exist locally or remotely", stackName)
		}
		return stack, nil
	}

	return stack, nil
}

func (pp *providerPreparer) importRemoteStack(ctx context.Context, projectName, stackName string) (*stacks.StackParameters, error) {
	existingStacks, err := pp.collectPreviouslyDeployedStacks(ctx, projectName)
	if err != nil {
		return nil, fmt.Errorf("failed to collect existing stacks: %w", err)
	}
	for _, existingStack := range existingStacks {
		if existingStack.Name == stackName {
			return &existingStack.StackParameters, nil
		}
	}

	return nil, fmt.Errorf("stack %q does not exist remotely", stackName)
}

func (pp *providerPreparer) promptForStackParameters(ctx context.Context) (*stacks.StackParameters, error) {
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

	return &params, nil
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
	if os.Getenv("AWS_PROFILE") != "" {
		return nil
	}

	if os.Getenv("AWS_ACCESS_KEY_ID") != "" && os.Getenv("AWS_SECRET_ACCESS_KEY") != "" {
		return nil
	}

	// TODO: add support for aws sso strategy
	knownProfiles, err := listAWSProfiles()
	if err != nil {
		return fmt.Errorf("failed to list AWS profiles: %w", err)
	}
	if len(knownProfiles) > 0 {
		const useAccessKeysOption = "Use Access Key ID and Secret Access Key"
		knownProfiles = append(knownProfiles, useAccessKeysOption)
		profile, err := pp.ec.RequestEnum(ctx, "Select your profile", "profile_name", knownProfiles)
		if err != nil {
			return fmt.Errorf("failed to elicit AWS Profile Name: %w", err)
		}
		if profile != useAccessKeysOption {
			err := os.Setenv("AWS_PROFILE", profile)
			if err != nil {
				return fmt.Errorf("failed to set AWS_PROFILE environment variable: %w", err)
			}
			return nil
		}
	}
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
		if err := scanner.Err(); err != nil {
			f.Close()
			return nil, fmt.Errorf("error reading %s: %w", file, err)
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
