package stacks

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
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

func (w *Wizard) CollectParameters(ctx context.Context) (*Parameters, error) {
	return w.CollectRemainingParameters(ctx, &Parameters{})
}

func (w *Wizard) CollectRemainingParameters(ctx context.Context, params *Parameters) (*Parameters, error) {
	// Initialize Variables map if nil
	if params.Variables == nil {
		params.Variables = make(map[string]string)
	}

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

	// Clear region for Defang provider as it doesn't use regions
	if params.Provider == client.ProviderDefang {
		params.Region = ""
	} else if params.Region == "" {
		defaultRegion := client.GetRegion(params.Provider)
		region, err := w.ec.RequestStringWithOptions(ctx, "Which region do you want to deploy to?", "region",
			elicitations.WithDefault(defaultRegion),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to elicit region choice: %w", err)
		}
		params.Region = region
	}

	if params.Name == "" {
		defaultName := MakeDefaultName(params.Provider, params.Region)
		name, err := w.ec.RequestStringWithOptions(ctx, "What do you want to call this stack?:", "stack_name",
			elicitations.WithDefault(defaultName),
			elicitations.WithValidator(ValidStackName),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to elicit stack name: %w", err)
		}

		params.Name = name
	}

	switch params.Provider {
	case client.ProviderAWS:
		if params.Variables["AWS_PROFILE"] == "" {
			if os.Getenv("AWS_PROFILE") != "" {
				profile, err := w.ec.RequestStringWithOptions(ctx, "Which AWS profile do you want to use?", "aws_profile",
					elicitations.WithDefault(os.Getenv("AWS_PROFILE")),
				)
				if err != nil {
					return nil, fmt.Errorf("failed to elicit AWS profile: %w", err)
				}
				params.Variables["AWS_PROFILE"] = profile
				break
			}
			profiles, err := w.profileLister.ListProfiles()
			if err != nil || len(profiles) == 0 {
				profile, err := w.ec.RequestStringWithOptions(ctx, "Which AWS profile do you want to use?", "aws_profile",
					elicitations.WithDefault("default"),
				)
				if err != nil {
					return nil, fmt.Errorf("failed to elicit AWS profile: %w", err)
				}
				params.Variables["AWS_PROFILE"] = profile
			} else {
				profile, err := w.ec.RequestEnum(ctx, "Which AWS profile do you want to use?", "aws_profile", profiles)
				if err != nil {
					return nil, fmt.Errorf("failed to elicit AWS profile: %w", err)
				}
				params.Variables["AWS_PROFILE"] = profile
			}
		}
	case client.ProviderGCP:
		if params.Variables["GCP_PROJECT_ID"] == "" {
			_, envProjectID := pkg.GetFirstEnv(pkg.GCPProjectEnvVars...)
			if envProjectID != "" {
				projectID, err := w.ec.RequestStringWithOptions(ctx, "What is your GCP Project ID?:", "gcp_project_id",
					elicitations.WithDefault(envProjectID),
				)
				if err != nil {
					return nil, fmt.Errorf("failed to elicit GCP Project ID: %w", err)
				}
				params.Variables["GCP_PROJECT_ID"] = projectID
				break
			}
			projectID, err := w.ec.RequestStringWithOptions(ctx, "What is your GCP Project ID?:", "gcp_project_id")
			if err != nil {
				return nil, fmt.Errorf("failed to elicit GCP Project ID: %w", err)
			}
			params.Variables["GCP_PROJECT_ID"] = projectID
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

func ValidStackName(val any) error {
	// the reflect value of the result
	value := reflect.ValueOf(val)
	str, ok := value.Interface().(string)
	if !ok {
		return errors.New("Value is required")
	}
	if len(str) == 0 {
		return errors.New("Value cannot be empty")
	}

	// if the value starts with a number, return an error
	firstChar := str[0]
	if firstChar >= '0' && firstChar <= '9' {
		return errors.New("Value must not start with a number")
	}

	// if the value is not alphanumeric return an error
	for _, r := range str {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
			return errors.New("Value must be alphanumeric")
		}
	}

	return nil
}
