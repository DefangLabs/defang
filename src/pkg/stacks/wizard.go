package stacks

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/elicitations"
)

type Wizard struct {
	ec            elicitations.Controller
	profileLister AWSProfileLister
}

func NewWizard(ec elicitations.Controller) *Wizard {
	return &Wizard{
		ec:            ec,
		profileLister: &FileSystemAWSProfileLister{},
	}
}

func NewWizardWithProfileLister(ec elicitations.Controller, profileLister AWSProfileLister) *Wizard {
	return &Wizard{
		ec:            ec,
		profileLister: profileLister,
	}
}

func (w *Wizard) CollectParameters(ctx context.Context) (*StackParameters, error) {
	return w.CollectRemainingParameters(ctx, &StackParameters{})
}

func (w *Wizard) CollectRemainingParameters(ctx context.Context, params *StackParameters) (*StackParameters, error) {
	if params.Provider == client.ProviderAuto || params.Provider == "" {
		var providerNames []string
		for _, p := range client.AllProviders() {
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

		var providerID client.ProviderID
		err = providerID.Set(providerName)
		if err != nil {
			return nil, err
		}
		params.Provider = providerID
	}

	if params.Provider == client.ProviderDefang {
		params.Region = ""
	} else if params.Region == "" {
		defaultRegion := client.GetRegion(params.Provider)
		region, err := w.ec.RequestStringWithDefault(ctx, "Which region do you want to deploy to?", "region", defaultRegion)
		if err != nil {
			return nil, fmt.Errorf("failed to elicit region choice: %w", err)
		}
		params.Region = region
	}

	if params.Name == "" {
		defaultName := MakeDefaultName(params.Provider, params.Region)
		name, err := w.ec.RequestStringWithDefault(ctx, "Enter a name for your stack:", "stack_name", defaultName)
		if err != nil {
			return nil, fmt.Errorf("failed to elicit stack name: %w", err)
		}

		params.Name = name
	}

	switch params.Provider {
	case client.ProviderAWS:
		if params.AWSProfile == "" {
			if os.Getenv("AWS_PROFILE") != "" {
				profile, err := w.ec.RequestStringWithDefault(ctx, "Which AWS profile do you want to use?", "aws_profile", os.Getenv("AWS_PROFILE"))
				if err != nil {
					return nil, fmt.Errorf("failed to elicit AWS profile: %w", err)
				}
				params.AWSProfile = profile
				break
			}
			profiles, err := w.profileLister.ListProfiles()
			if err != nil || len(profiles) == 0 {
				profile, err := w.ec.RequestStringWithDefault(ctx, "Enter the AWS profile you want to use:", "aws_profile", "default")
				if err != nil {
					return nil, fmt.Errorf("failed to elicit AWS profile: %w", err)
				}
				params.AWSProfile = profile
			} else {
				profile, err := w.ec.RequestEnum(ctx, "Which AWS profile do you want to use?", "aws_profile", profiles)
				if err != nil {
					return nil, fmt.Errorf("failed to elicit AWS profile: %w", err)
				}
				params.AWSProfile = profile
			}
		}
	case client.ProviderGCP:
		if params.GCPProjectID == "" {
			// Check all supported GCP project environment variables
			envProjectID := pkg.GetFirstEnv(pkg.GCPProjectEnvVars...)
			if envProjectID != "" {
				projectID, err := w.ec.RequestStringWithDefault(ctx, "Enter your GCP Project ID:", "gcp_project_id", envProjectID)
				if err != nil {
					return nil, fmt.Errorf("failed to elicit GCP Project ID: %w", err)
				}
				params.GCPProjectID = projectID
				break
			}
			projectID, err := w.ec.RequestString(ctx, "Enter your GCP Project ID:", "gcp_project_id")
			if err != nil {
				return nil, fmt.Errorf("failed to elicit GCP Project ID: %w", err)
			}
			params.GCPProjectID = projectID
		}
	}

	return params, nil
}

type AWSProfileLister interface {
	ListProfiles() ([]string, error)
}

type FileSystemAWSProfileLister struct{}

func (f *FileSystemAWSProfileLister) ListProfiles() ([]string, error) {
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
