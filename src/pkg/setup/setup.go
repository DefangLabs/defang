package setup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/login"
	"github.com/DefangLabs/defang/src/pkg/migrate"
	"github.com/DefangLabs/defang/src/pkg/surveyor"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/track"
	"github.com/bufbuild/connect-go"
)

var P = track.P

type SetupClient struct {
	Surveyor   *surveyor.DefaultSurveyor
	ModelID    string
	Fabric     client.FabricClient
	FabricAddr string
}

func (s *SetupClient) Start(ctx context.Context) (SetupResult, error) {
	var response string
	err := s.Surveyor.AskOne(&survey.Select{
		Message: "How would you like to start?",
		Options: []string{
			"Generate with AI",
			"Clone a sample",
			"Migrate from Heroku",
		},
	}, &response)
	if err != nil {
		return SetupResult{}, fmt.Errorf("failed to ask how to start: %w", err)
	}

	switch response {
	case "Generate with AI":
		return s.AIGenerate(ctx)
	case "Clone a sample":
		return s.CloneSample(ctx, "")
	case "Migrate from Heroku":
		return s.MigrateFromHeroku(ctx)
	}

	return SetupResult{}, errors.New("invalid option selected")
}

const generateWithAI = "Generate with AI"

func promptForSample(ctx context.Context) (string, error) {
	sampleList, err := cli.FetchSamples(ctx)
	// Fetch the list of samples from the Defang repository
	if err != nil {
		return "", fmt.Errorf("unable to fetch samples: %w", err)
	}
	if len(sampleList) == 0 {
		return "", errors.New("no samples available")
	}

	sample := ""
	sampleNames := []string{generateWithAI}
	sampleTitles := []string{"Generate a sample from scratch using a language prompt"}
	sampleIndex := []string{"unused first entry because we always show genAI option"}
	for _, s := range sampleList {
		sampleNames = append(sampleNames, s.Name)
		sampleTitles = append(sampleTitles, s.Title)
		sampleIndex = append(sampleIndex, strings.ToLower(s.Name+" "+s.Title+" "+
			strings.Join(s.Tags, " ")+" "+strings.Join(s.Languages, " ")))
	}

	err = survey.AskOne(&survey.Select{
		Message: "Choose a sample service:",
		Options: sampleNames,
		Help:    "The project code will be based on the sample you choose here.",
		Filter: func(filter string, value string, i int) bool {
			return i == 0 || strings.Contains(sampleIndex[i], strings.ToLower(filter))
		},
		Description: func(value string, i int) string {
			return sampleTitles[i]
		},
	}, &sample, survey.WithStdio(term.DefaultTerm.Stdio()))
	if err != nil {
		return "", fmt.Errorf("failed to select sample: %w", err)
	}

	return sample, nil
}

var defaultFolder = "project1"

const GenerateStartedEvt = "Generate Started"

func (s *SetupClient) AIGenerate(ctx context.Context) (SetupResult, error) {
	prompt := GeneratePrompt{
		ModelID: s.ModelID,
	}
	var qs = []*survey.Question{
		{
			Name: "language",
			Prompt: &survey.Select{
				Message: "Choose the language you'd like to use:",
				Options: cli.SupportedLanguages,
				Help:    "The project code will be in the language you choose here.",
			},
		},
		{
			Name: "description",
			Prompt: &survey.Input{
				Message: "Please describe the service you'd like to build:",
				Help: `Here are some example prompts you can use:
    "A simple 'hello world' function"
    "A service with 2 endpoints, one to upload and the other to download a file from AWS S3"
    "A service with a default endpoint that returns an HTML page with a form asking for the user's name and then a POST endpoint to handle the form post when the user clicks the 'submit' button"`,
			},
			Validate: survey.MinLength(5),
		},
	}
	err := survey.Ask(qs, &prompt, survey.WithStdio(term.DefaultTerm.Stdio()))
	if err != nil {
		return SetupResult{}, fmt.Errorf("failed to prompt for AI generation: %w", err)
	}
	folder, err := promptForDirectory(defaultFolder)
	if err != nil {
		return SetupResult{}, err
	}
	if s.Fabric.CheckLoginAndToS(ctx) != nil {
		// The user is either not logged in or has not agreed to the terms of service; ask for agreement to the terms now
		if err := login.InteractiveAgreeToS(ctx, s.Fabric); err != nil {
			// This might fail because the user did not log in. This is fine: server won't save the terms agreement, but can proceed with the generation
			if connect.CodeOf(err) != connect.CodeUnauthenticated {
				return SetupResult{}, err
			}
		}
	}

	track.Evt(GenerateStartedEvt, P("language", prompt.Language), P("description", prompt.Description), P("folder", folder), P("model", prompt.ModelID))
	beforeGenerate(folder)
	term.Info("Working on it. This may take 1 or 2 minutes...")
	args := cli.GenerateArgs{
		Description: prompt.Description,
		Folder:      folder,
		Language:    prompt.Language,
		ModelId:     prompt.ModelID,
	}
	_, err = cli.GenerateWithAI(ctx, s.Fabric, args)
	if err != nil {
		return SetupResult{}, err
	}
	return SetupResult{Folder: folder}, nil
}

type SetupResult struct {
	Folder string
}

func (s *SetupClient) CloneSample(ctx context.Context, sample string) (SetupResult, error) {
	var err error
	if sample == "" {
		sample, err = promptForSample(ctx)
		if err != nil {
			return SetupResult{}, fmt.Errorf("failed to prompt for sample: %w", err)
		}
	}
	if sample == generateWithAI {
		return s.AIGenerate(ctx)
	}

	folder, err := promptForDirectory(sample)
	if err != nil {
		return SetupResult{}, err
	}
	track.Evt(GenerateStartedEvt, P("sample", sample), P("folder", folder))
	beforeGenerate(folder)
	term.Info("Fetching sample from the Defang repository...")
	err = cli.InitFromSamples(ctx, folder, []string{sample})
	if err != nil {
		return SetupResult{}, err
	}

	return SetupResult{Folder: folder}, nil
}

func promptForDirectory(defaultDirectory string) (string, error) {
	var folder string
	err := survey.AskOne(&survey.Input{
		Message: "What folder would you like to create the project in?",
		Default: defaultDirectory,
		Help:    "The generated code will be in the folder you choose here. If the folder does not exist, it will be created.",
	}, &folder, survey.WithStdio(term.DefaultTerm.Stdio()))
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(folder), nil
}

type GeneratePrompt struct {
	Description string `json:"description"`
	ModelID     string `json:"model_id"`
	Language    string `json:"language"`
}

func beforeGenerate(directory string) {
	// Check if the current folder is empty
	if empty, err := pkg.IsDirEmpty(directory); !os.IsNotExist(err) && !empty {
		nonEmptyFolder := fmt.Sprintf("The folder %q is not empty. We recommend running this command in an empty folder.", directory)

		var confirm bool
		err := survey.AskOne(&survey.Confirm{
			Message: nonEmptyFolder + " Continue creating project?",
		}, &confirm, survey.WithStdio(term.DefaultTerm.Stdio()))
		if err == nil && !confirm {
			os.Exit(1)
		}
	}
}

func (s *SetupClient) MigrateFromHeroku(ctx context.Context) (SetupResult, error) {
	var err error
	s.Fabric, err = login.InteractiveRequireLoginAndToS(ctx, s.Fabric, s.FabricAddr)
	if err != nil {
		return SetupResult{}, err
	}

	term.Info("Ok, let's create a compose file for your existing deployment.")
	heroku := migrate.NewHerokuClient()
	composeFileContents, err := migrate.InteractiveSetup(ctx, s.Fabric, s.Surveyor, heroku, migrate.SourcePlatformHeroku)
	if err != nil {
		return SetupResult{}, err
	}

	composeFilePath, err := writeComposeFile(composeFileContents)
	if err != nil {
		return SetupResult{}, fmt.Errorf("failed to write compose file: %w", err)
	}

	term.Info("Compose file written to", composeFilePath)
	term.Info("Your application is now ready to deploy with Defang.")
	term.Info("For next steps, visit https://s.defang.io/from-heroku")

	return SetupResult{Folder: "."}, nil
}

func writeComposeFile(content string) (string, error) {
	paths := []string{"compose.yaml", "compose.defang.yaml"}
	var f *os.File
	var err error

	for _, composeFilePath := range paths {
		f, err = os.OpenFile(composeFilePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if err != nil {
			if os.IsExist(err) {
				continue
			}
			return "", fmt.Errorf("failed to create compose file: %w", err)
		}
		defer f.Close()

		// #nosec G306 -- compose file is not expected to contain sensitive data
		if _, err := f.WriteString(content); err != nil {
			return "", fmt.Errorf("failed to write compose file: %w", err)
		}
		return composeFilePath, nil
	}

	return "", fmt.Errorf("all compose file names already exist: %v", paths)
}
