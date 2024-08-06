package compose

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/DefangLabs/defang/src/pkg/logs"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
	"github.com/compose-spec/compose-go/v2/cli"
	"github.com/compose-spec/compose-go/v2/errdefs"
	compose "github.com/compose-spec/compose-go/v2/types"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

type LoaderOptions struct {
	ConfigPaths []string
	WorkingDir  string
}

type Loader struct {
	options LoaderOptions
}

func NewLoaderWithOptions(options LoaderOptions) Loader {
	return Loader{options: options}
}

func NewLoaderWithPath(path string) Loader {
	configPaths := []string{}
	if path != "" {
		configPaths = append(configPaths, path)
	}
	return NewLoaderWithOptions(LoaderOptions{ConfigPaths: configPaths})
}

func (c Loader) LoadCompose(ctx context.Context) (*compose.Project, error) {
	// Set logrus send logs via the term package
	termLogger := logs.TermLogFormatter{Term: term.DefaultTerm}
	logrus.SetFormatter(termLogger)

	projOpts, err := c.projectOptions()
	if err != nil {
		return nil, err
	}

	// HACK: We do not want to include all the os environment variables, only COMPOSE_PROJECT_NAME
	if envProjName, ok := os.LookupEnv("COMPOSE_PROJECT_NAME"); ok {
		projOpts.Environment["COMPOSE_PROJECT_NAME"] = envProjName
	}

	project, err := projOpts.LoadProject(ctx)
	if err != nil {
		if errors.Is(err, errdefs.ErrNotFound) {
			return nil, types.ErrComposeFileNotFound
		}

		return nil, err
	}

	if term.DoDebug() {
		b, _ := yaml.Marshal(project)
		fmt.Println(string(b))
	}

	return project, nil
}

func (c *Loader) projectOptions() (*cli.ProjectOptions, error) {
	options := c.options
	// Based on how docker compose setup its own project options
	// https://github.com/docker/compose/blob/1a14fcb1e6645dd92f5a4f2da00071bd59c2e887/cmd/compose/compose.go#L326-L346
	optFns := []cli.ProjectOptionsFn{
		cli.WithWorkingDirectory(options.WorkingDir),
		// First apply os.Environment, always win
		// -- DISABLED -- cli.WithOsEnv,
		// Load PWD/.env if present and no explicit --env-file has been set
		cli.WithEnvFiles(), // TODO: Support --env-file to be added as param to this call
		// read dot env file to populate project environment
		cli.WithDotEnv,
		// get compose file path set by COMPOSE_FILE
		cli.WithConfigFileEnv,
		// if none was selected, get default compose.yaml file from current dir or parent folder
		cli.WithDefaultConfigPath,
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

	return cli.NewProjectOptions(options.ConfigPaths, optFns...)
}
