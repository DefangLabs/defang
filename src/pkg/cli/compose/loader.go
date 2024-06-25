package compose

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
	"github.com/compose-spec/compose-go/v2/cli"
	"github.com/compose-spec/compose-go/v2/consts"
	compose "github.com/compose-spec/compose-go/v2/types"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

type Loader struct {
	ComposeFilePath string
}

func (c Loader) LoadCompose(ctx context.Context) (*compose.Project, error) {
	composeFilePath, err := getComposeFilePath(c.ComposeFilePath)
	if err != nil {
		return nil, err
	}
	term.Debugf("Loading compose file %q", composeFilePath)

	// Compose-go uses the logrus logger, so we need to configure it to be more like our own logger
	logrus.SetFormatter(&logrus.TextFormatter{DisableTimestamp: true, DisableColors: !term.StderrCanColor(), DisableLevelTruncation: true})

	projOpts, err := getDefaultProjectOptions(composeFilePath)
	if err != nil {
		return nil, err
	}

	project, err := projOpts.LoadProject(ctx)
	if err != nil {
		return nil, err
	}

	// Hack: Fill in the missing environment variables that were stripped by the normalization process
	projOpts, err = getDefaultProjectOptions(composeFilePath, cli.WithNormalization(false)) // Disable normalization to keep unset environment variables
	if err != nil {
		return nil, err
	}

	// Disable logrus output to prevent double warnings from compose-go
	currentOutput := logrus.StandardLogger().Out
	logrus.SetOutput(io.Discard)
	rawProj, err := projOpts.LoadProject(ctx)
	logrus.SetOutput(currentOutput)
	if err != nil {
		panic(err) // there's no good reason this should fail, since we've already loaded the project before
	}

	// TODO: Remove this hack once the PR is merged
	// PR Filed: https://github.com/compose-spec/compose-go/pull/634
	for name, rawService := range rawProj.Services {
		for key, value := range rawService.Environment {
			service := project.Services[name]
			if service.Environment[key] == nil {
				if service.Environment == nil {
					service.Environment = make(map[string]*string)
				}
				service.Environment[key] = value
				project.Services[name] = service
			}
		}
	}

	if term.DoDebug() {
		b, _ := yaml.Marshal(project)
		fmt.Println(string(b))
	}
	return project, nil
}

func getDefaultProjectOptions(composeFilePath string, extraOpts ...cli.ProjectOptionsFn) (*cli.ProjectOptions, error) {
	workingDir := filepath.Dir(composeFilePath)

	// Based on how docker compose setup its own project options
	// https://github.com/docker/compose/blob/1a14fcb1e6645dd92f5a4f2da00071bd59c2e887/cmd/compose/compose.go#L326-L346
	opts := []cli.ProjectOptionsFn{
		cli.WithWorkingDirectory(workingDir),
		// First apply os.Environment, always win
		// -- DISABLED -- cli.WithOsEnv,
		// Load PWD/.env if present and no explicit --env-file has been set
		cli.WithEnvFiles(), // TODO: Support --env-file to be added as param to this call
		// read dot env file to populate project environment
		cli.WithDotEnv,
		// get compose file path set by COMPOSE_FILE
		// cli.WithConfigFileEnv, not used as we ignore os.Environment
		// if none was selected, get default compose.yaml file from current dir or parent folder
		// // cli.WithDefaultConfigPath, NO: this ends up picking the "first" when more than one file is found NO: this ends up picking the "first" when more than one file is found
		cli.WithName(os.Getenv(consts.ComposeProjectName)), // HACK: We do not want to include all the os environment variables, only COMPOSE_PROJECT_NAME

		// Calling the 2 functions below the 2nd time as the loaded env in first call modifies the behavior of the 2nd call
		// .. and then, a project directory != PWD maybe has been set so let's load .env file
		cli.WithEnvFiles(), // TODO: Support --env-file to be added as param to this call
		cli.WithDotEnv,

		// DEFANG SPECIFIC OPTIONS
		cli.WithDefaultProfiles("defang"),
		cli.WithDiscardEnvFile,
		cli.WithConsistency(false), // TODO: check fails if secrets are used but top-level 'secrets:' is missing
	}
	opts = append(opts, extraOpts...)
	projOpts, err := cli.NewProjectOptions([]string{composeFilePath}, opts...)
	if err != nil {
		return nil, err
	}

	return projOpts, nil
}

func getComposeFilePath(userSpecifiedComposeFile string) (string, error) {
	// The Compose file is compose.yaml (preferred) or compose.yml that is placed in the current directory or higher.
	// Compose also supports docker-compose.yaml and docker-compose.yml for backwards compatibility.
	// Users can override the file by specifying file name
	const DEFAULT_COMPOSE_FILE_PATTERN = "*compose.y*ml"

	path, err := os.Getwd()
	if err != nil {
		return path, err
	}

	searchPattern := DEFAULT_COMPOSE_FILE_PATTERN
	if len(userSpecifiedComposeFile) > 0 {
		path = ""
		searchPattern = userSpecifiedComposeFile
	}

	// iterate through this loop at least once to find the compose file.
	// if the user did not specify a specific file (i.e. userSpecifiedComposeFile == "")
	// then walk the tree up to the root directory looking for a compose file.
	term.Debug("Looking for compose file - searching for", searchPattern)
	for {
		if files, _ := filepath.Glob(filepath.Join(path, searchPattern)); len(files) > 1 {
			return "", fmt.Errorf("multiple Compose files found: %q; use -f to specify which one to use", files)
		} else if len(files) == 1 {
			// found compose file, we're done
			return files[0], nil
		}

		if len(userSpecifiedComposeFile) > 0 {
			return "", fmt.Errorf("no Compose file found at %q: %w", userSpecifiedComposeFile, os.ErrNotExist)
		}

		// compose file not found, try parent directory
		nextPath := filepath.Dir(path)
		if nextPath == path {
			// previous search was of root, we're done
			return "", types.ErrComposeFileNotFound
		}

		path = nextPath
	}
}
