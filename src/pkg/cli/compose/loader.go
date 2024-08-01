package compose

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/DefangLabs/defang/src/pkg/logs"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
	"github.com/compose-spec/compose-go/v2/cli"
	compose "github.com/compose-spec/compose-go/v2/types"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

type Loader struct {
	ComposeFilePath string
}

func NewLoader(composeFilePath string) (*Loader, error) {
	var err error

	if composeFilePath == "" {
		composeFilePath, err = findDefaultComposeFilePath()
		if err != nil {
			return nil, types.ErrComposeFileNotFound
		}

		workingDir, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		err = warnMultipleComposeFiles(workingDir)
		if err != nil {
			return nil, err
		}
	}

	composeFilePath, err = filepath.Abs(composeFilePath)
	if err != nil {
		return nil, err
	}

	return &Loader{ComposeFilePath: composeFilePath}, nil
}

func (c Loader) LoadCompose(ctx context.Context) (*compose.Project, error) {
	// Set logrus send logs via the term package
	termLogger := logs.TermLogFormatter{Term: term.DefaultTerm}
	logrus.SetFormatter(termLogger)

	projOpts, err := getDefaultProjectOptions(c.ComposeFilePath)
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

	if term.DoDebug() {
		b, _ := yaml.Marshal(project)
		fmt.Println(string(b))
	}
	return project, nil
}

func warnMultipleComposeFiles(workingDir string) error {
	count := 0
	for _, file := range cli.DefaultFileNames {
		if _, err := os.Stat(filepath.Join(workingDir, file)); err == nil {
			count++
		}
	}

	if count > 1 {
		return types.ErrMultipleComposeFilesFound
	}

	return nil
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
		cli.WithConfigFileEnv,
		// if none was selected, get default compose.yaml file from current dir or parent folder
		// cli.WithDefaultConfigPath, // NO: we find the config path eagerly when setting up the Loader
		// cli.WithName(o.ProjectName)

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

func findDefaultComposeFilePath() (string, error) {
	opts := []cli.ProjectOptionsFn{
		cli.WithDefaultConfigPath,
	}
	projOpts, err := cli.NewProjectOptions([]string{}, opts...)
	if err != nil {
		return "", err
	}

	// TODO: add support for default override files
	return projOpts.ConfigPaths[0], nil
}
