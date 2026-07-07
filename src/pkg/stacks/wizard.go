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
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

// RecipeLister fetches the recipes available in the workspace. It is satisfied
// by client.FabricClient and the stacks Lister.
type RecipeLister interface {
	ListRecipes(ctx context.Context) (*defangv1.ListRecipesResponse, error)
}

type Wizard struct {
	ec            elicitations.Controller
	recipeLister  RecipeLister
	profileLister AWSProfileLister
}

func NewWizard(ec elicitations.Controller, recipeLister RecipeLister) *Wizard {
	return &Wizard{
		ec:            ec,
		recipeLister:  recipeLister,
		profileLister: &FileSystemAWSProfileLister{},
	}
}

func NewWizardWithProfileLister(ec elicitations.Controller, recipeLister RecipeLister, profileLister AWSProfileLister) *Wizard {
	return &Wizard{
		ec:            ec,
		recipeLister:  recipeLister,
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
		for _, p := range client.ByocProviders() {
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
		region, err := w.ec.RequestString(ctx, "Which region do you want to deploy to?", "region",
			elicitations.WithDefault(defaultRegion),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to elicit region choice: %w", err)
		}
		params.Region = region
	}

	if params.Recipe == modes.RecipeUnspecified && params.Provider != client.ProviderDefang {
		// Skip the prompt entirely when Fabric reports no active recipes.
		if recipeNames := w.activeRecipeNames(ctx); len(recipeNames) > 0 {
			recipeName, err := w.ec.RequestEnum(ctx, "Which recipe (deployment mode) do you want to deploy with?", "recipe",
				recipeNames,
			)
			if err != nil {
				return nil, fmt.Errorf("failed to elicit deployment mode: %w", err)
			}
			params.Recipe = modes.ParseRecipe(recipeName)
		}
	}

	if params.Name == "" {
		defaultName := MakeDefaultName(params.Provider, params.Region)
		name, err := w.ec.RequestString(ctx, "What do you want to call this stack?:", "stack_name",
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
				profile, err := w.ec.RequestString(ctx, "Which AWS profile do you want to use?", "aws_profile",
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
				profile, err := w.ec.RequestString(ctx, "Which AWS profile do you want to use?", "aws_profile",
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
				projectID, err := w.ec.RequestString(ctx, "What is your GCP Project ID?:", "gcp_project_id",
					elicitations.WithDefault(envProjectID),
				)
				if err != nil {
					return nil, fmt.Errorf("failed to elicit GCP Project ID: %w", err)
				}
				params.Variables["GCP_PROJECT_ID"] = projectID
				break
			}
			projectID, err := w.ec.RequestString(ctx, "What is your GCP Project ID?:", "gcp_project_id")
			if err != nil {
				return nil, fmt.Errorf("failed to elicit GCP Project ID: %w", err)
			}
			params.Variables["GCP_PROJECT_ID"] = projectID
		}
	case client.ProviderAzure:
		if params.Variables["AZURE_SUBSCRIPTION_ID"] == "" {
			subscriptionID, err := w.ec.RequestString(ctx, "What is your Azure Subscription ID?:", "azure_subscription_id",
				elicitations.WithDefault(os.Getenv("AZURE_SUBSCRIPTION_ID")),
			)
			if err != nil {
				return nil, fmt.Errorf("failed to elicit Azure Subscription ID: %w", err)
			}
			params.Variables["AZURE_SUBSCRIPTION_ID"] = subscriptionID
		}
	}

	return params, nil
}

// activeRecipeNames returns the recipe names to offer in the wizard, fetched
// from Fabric. When no recipe lister is configured or Fabric is unreachable it
// falls back to the (non-empty) built-in deployment modes. When Fabric reports
// zero active recipes it returns an empty slice, signalling the caller to skip
// the recipe prompt.
func (w *Wizard) activeRecipeNames(ctx context.Context) []string {
	if w.recipeLister == nil {
		return modes.AllDeploymentModes()
	}
	resp, err := w.recipeLister.ListRecipes(ctx)
	if err != nil {
		term.Debugf("could not list recipes, falling back to built-in modes: %v", err)
		return modes.AllDeploymentModes()
	}
	var names []string
	for _, recipe := range resp.GetRecipes() {
		if recipe.GetActive() {
			names = append(names, recipe.GetName())
		}
	}
	sort.Strings(names)
	return names
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
		if err := scanner.Err(); err != nil {
			continue // skip files with read errors
		}
	}

	result := make([]string, 0, len(profiles))
	for p := range profiles {
		result = append(result, p)
	}
	sort.Strings(result)
	return result, nil
}

// this is an elicitations validator function
func ValidStackName(val any) error {
	value := reflect.ValueOf(val)
	str, ok := value.Interface().(string)
	if !ok {
		return errors.New("Value is required")
	}
	if len(str) == 0 {
		return errors.New("Value cannot be empty")
	}

	return ValidateStackName(str)
}
