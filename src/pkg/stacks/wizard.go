package stacks

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/elicitations"
)

type Wizard struct {
	ec elicitations.Controller
}

func NewWizard(ec elicitations.Controller) *Wizard {
	return &Wizard{
		ec: ec,
	}
}

func (w *Wizard) CollectParameters(ctx context.Context) (*StackParameters, error) {
	var providerNames []string
	for _, p := range cliClient.AllProviders() {
		providerNames = append(providerNames, p.Name())
	}
	providerName, err := w.ec.RequestEnum(
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
		region, err = w.ec.RequestStringWithDefault(ctx, "Which region do you want to deploy to?", "region", defaultRegion)
		if err != nil {
			return nil, fmt.Errorf("failed to elicit region choice: %w", err)
		}
	}

	defaultName := MakeDefaultName(providerID, region)
	name, err := w.ec.RequestStringWithDefault(ctx, "Enter a name for your stack:", "stack_name", defaultName)
	if err != nil {
		return nil, fmt.Errorf("failed to elicit stack name: %w", err)
	}
	params := StackParameters{
		Provider: providerID,
		Region:   region,
		Name:     name,
	}

	switch providerID {
	case cliClient.ProviderAWS:
		var profile string
		if os.Getenv("AWS_PROFILE") != "" {
			profile, err = w.ec.RequestStringWithDefault(ctx, "Which AWS profile do you want to use?", "aws_profile", os.Getenv("AWS_PROFILE"))
			if err != nil {
				return nil, fmt.Errorf("failed to elicit AWS profile: %w", err)
			}
			params.AWSProfile = profile
			break
		}
		profiles, err := listAWSProfiles()
		if err != nil {
			profile, err = w.ec.RequestStringWithDefault(ctx, "Enter the AWS profile you want to use:", "aws_profile", "default")
			if err != nil {
				return nil, fmt.Errorf("failed to elicit AWS profile: %w", err)
			}
		} else {
			profile, err = w.ec.RequestEnum(ctx, "Which AWS profile do you want to use?", "aws_profile", profiles)
			if err != nil {
				return nil, fmt.Errorf("failed to elicit AWS profile: %w", err)
			}
		}
		params.AWSProfile = profile
	case cliClient.ProviderGCP:
		var projectID string
		if os.Getenv("GCP_PROJECT_ID") != "" {
			projectID, err = w.ec.RequestStringWithDefault(ctx, "Enter your GCP Project ID:", "gcp_project_id", os.Getenv("GCP_PROJECT_ID"))
			if err != nil {
				return nil, fmt.Errorf("failed to elicit GCP Project ID: %w", err)
			}
			params.GCPProjectID = projectID
			break
		}
		projectID, err = w.ec.RequestString(ctx, "Enter your GCP Project ID:", "gcp_project_id")
		if err != nil {
			return nil, fmt.Errorf("failed to elicit GCP Project ID: %w", err)
		}
		params.GCPProjectID = projectID
	}

	return &params, nil
}

func listAWSProfiles() ([]string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	files := []string{
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
