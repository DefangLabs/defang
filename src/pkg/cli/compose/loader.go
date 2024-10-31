package compose

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"

	"github.com/DefangLabs/defang/src/pkg/logs"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
	"github.com/compose-spec/compose-go/v2/cli"
	"github.com/compose-spec/compose-go/v2/errdefs"
	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/template"
	composeTypes "github.com/compose-spec/compose-go/v2/types"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

type Project = composeTypes.Project

type ServiceConfig = composeTypes.ServiceConfig

type Services = composeTypes.Services

type LoaderOptions struct {
	ConfigPaths []string
	ProjectName string
}

type Loader struct {
	options LoaderOptions
}

func NewLoaderWithOptions(options LoaderOptions) Loader {
	// if no --project-name is provided, try to get it from the environment
	// https://docs.docker.com/compose/project-name/#set-a-project-name
	if options.ProjectName == "" {
		if envProjName, ok := os.LookupEnv("COMPOSE_PROJECT_NAME"); ok {
			options.ProjectName = envProjName
		}
	}

	return Loader{options: options}
}

func NewLoaderWithPath(path string) Loader {
	configPaths := []string{}
	if path != "" {
		configPaths = append(configPaths, path)
	}
	return NewLoaderWithOptions(LoaderOptions{ConfigPaths: configPaths})
}

func (c Loader) LoadProjectName(ctx context.Context) (string, error) {
	if c.options.ProjectName != "" {
		return c.options.ProjectName, nil
	}

	project, err := c.LoadProject(ctx)
	if err != nil {
		return "", err
	}

	return project.Name, nil
}

func (c Loader) LoadProject(ctx context.Context) (*Project, error) {
	// Set logrus send logs via the term package
	termLogger := logs.TermLogFormatter{Term: term.DefaultTerm}
	logrus.SetFormatter(termLogger)

	projOpts, err := c.newProjectOptions()
	if err != nil {
		return nil, err
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

func (c *Loader) newProjectOptions() (*cli.ProjectOptions, error) {
	// Based on how docker compose setup its own project options
	// https://github.com/docker/compose/blob/1a14fcb1e6645dd92f5a4f2da00071bd59c2e887/cmd/compose/compose.go#L326-L346
	optFns := []cli.ProjectOptionsFn{
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
		cli.WithName(c.options.ProjectName),

		// Calling the 2 functions below the 2nd time as the loaded env in first call modifies the behavior of the 2nd call
		// .. and then, a project directory != PWD maybe has been set so let's load .env file
		cli.WithEnvFiles(), // TODO: Support --env-file to be added as param to this call
		cli.WithDotEnv,

		// DEFANG SPECIFIC OPTIONS
		cli.WithDefaultProfiles("defang"),
		cli.WithDiscardEnvFile,
		cli.WithConsistency(false), // TODO: check fails if secrets are used but top-level 'secrets:' is missing
		cli.WithLoadOptions(func(o *loader.Options) {
			// Override the interpolation substitution function to leave unresolved variables as is for resolution later by CD
			o.Interpolate.Substitute = func(templ string, mapping template.Mapping) (string, error) {
				return template.Substitute(templ, func(key string) (string, bool) {
					if v, ok := mapping(key); ok {
						return v, true
					}
					// Check if the variable is defined in the environment to warn the user that it's not used
					_, inEnv := os.LookupEnv(key)
					if hasSubstitution(templ, key) {
						// We don't (yet) support substitution patterns during deployment
						if inEnv {
							term.Warnf("Environment variable %q is not used; add it to `.env` if needed", key)
						} else {
							term.Debugf("Unresolved variable %s", key)
						}
						return "", false
					}
					if inEnv {
						term.Warnf("Environment variable %q is not used; add it to `.env` or it may be resolved from config during deployment", key)
					} else {
						term.Debugf("Unresolved variable %q may be resolved from config during deployment", key)
					}
					// Leave unresolved variables as-is for resolution later by CD
					return "${" + key + "}", true
				})
			}
		}),
	}

	return cli.NewProjectOptions(c.options.ConfigPaths, optFns...)
}

func hasSubstitution(s, key string) bool {
	// Check in the original `templ` string if the variable uses any substitution patterns like - :- + :+ ? :?
	pattern := regexp.MustCompile(`(^|[^$])\$\{` + regexp.QuoteMeta(key) + `:?[-+?]`)
	return pattern.MatchString(s)
}

func LoadFromContent(ctx context.Context, content []byte) (*Project, error) {
	return loader.LoadWithContext(ctx, composeTypes.ConfigDetails{ConfigFiles: []composeTypes.ConfigFile{{Content: content}}}, func(o *loader.Options) {
		o.SkipConsistencyCheck = true // this matches the WithConsistency(false) option above
	})
}
