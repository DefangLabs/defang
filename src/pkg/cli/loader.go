package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/compose-spec/compose-go/v2/cli"
	compose "github.com/compose-spec/compose-go/v2/types"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

type ComposeLoader struct {
	ComposeFilePath string
}

func (c ComposeLoader) LoadCompose(ctx context.Context) (*compose.Project, error) {
	composeFilePath, err := getComposeFilePath(c.ComposeFilePath)
	if err != nil {
		return nil, err
	}
	term.Debug("Loading compose file", composeFilePath)
	workDir := filepath.Dir(composeFilePath)

	// Compose-go uses the logrus logger, so we need to configure it to be more like our own logger
	logrus.SetFormatter(&logrus.TextFormatter{DisableTimestamp: true, DisableColors: !term.StderrCanColor(), DisableLevelTruncation: true})

	projOpts, err := getDefaultProjectOptions(workDir)
	if err != nil {
		return nil, err
	}

	// HACK: We do not want to include all the os environment variables, only COMPOSE_PROJECT_NAME
	if envProjName, ok := os.LookupEnv("COMPOSE_PROJECT_NAME"); ok {
		projOpts.Environment["COMPOSE_PROJECT_NAME"] = envProjName
	}

	project, err := projOpts.LoadProject(ctx)
	if err != nil {
		return nil, err
	}

	// Hack: Fill in the missing environment variables that were stripped by the normalization process
	projOpts, err = getDefaultProjectOptions(workDir, cli.WithNormalization(false)) // Disable normalization to keep unset environment variables
	if err != nil {
		return nil, err
	}

	// Disable logrus output to prevent double warnings from compose-go
	currentOutput := logrus.StandardLogger().Out
	logrus.SetOutput(io.Discard)
	rawProj, err := projOpts.LoadProject(ctx)
	logrus.SetOutput(currentOutput)
	if err != nil {
		return nil, err // there's no good reason this should fail, since we've already loaded the project
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

func getDefaultProjectOptions(workingDir string, extraOpts ...cli.ProjectOptionsFn) (*cli.ProjectOptions, error) {
	// Based on how docker compose setup its own project options
	// https://github.com/docker/compose/blob/1a14fcb1e6645dd92f5a4f2da00071bd59c2e887/cmd/compose/compose.go#L326-L346
	opts := []cli.ProjectOptionsFn{
		cli.WithWorkingDirectory(workingDir),
		// First apply os.Environment, always win
		// -- DISABLED -- cli.WithOsEnv,
		// Load PWD/.env if present and no explicit --env-file has been set
		// -- NO SUCH PARAM YET -- cli.WithEnvFiles(o.EnvFiles...), TODO: Do we support env files?
		// read dot env file to populate project environment
		// -- DISABLED -- cli.WithDotEnv,
		// get compose file path set by COMPOSE_FILE
		cli.WithConfigFileEnv,
		// if none was selected, get default compose.yaml file from current dir or parent folder
		cli.WithDefaultConfigPath,
		// cli.WithName(o.ProjectName)

		// DEFANG SPECIFIC OPTIONS
		cli.WithDefaultProfiles("defang"),
		cli.WithDiscardEnvFile,
		cli.WithConsistency(false), // TODO: check fails if secrets are used but top-level 'secrets:' is missing
	}
	opts = append(opts, extraOpts...)
	projOpts, err := cli.NewProjectOptions(nil, opts...)
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
			err = fmt.Errorf("multiple Compose files found: %q; use -f to specify which one to use", files)
			break
		} else if len(files) == 1 {
			// found compose file, we're done
			path = files[0]
			break
		}

		if len(userSpecifiedComposeFile) > 0 {
			err = fmt.Errorf("no Compose file found at %q: %w", userSpecifiedComposeFile, os.ErrNotExist)
			break
		}

		// compose file not found, try parent directory
		nextPath := filepath.Dir(path)
		if nextPath == path {
			// previous search was of root, we're done
			err = fmt.Errorf("no Compose file found")
			break
		}

		path = nextPath
	}

	return path, err
}
