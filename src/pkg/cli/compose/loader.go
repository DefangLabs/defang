package compose

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/logs"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
	"github.com/compose-spec/compose-go/v2/cli"
	"github.com/compose-spec/compose-go/v2/errdefs"
	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/template"
	composeTypes "github.com/compose-spec/compose-go/v2/types"
	"github.com/sirupsen/logrus"
	"go.yaml.in/yaml/v3"
)

type Project = composeTypes.Project

type ServiceConfig = composeTypes.ServiceConfig

type Services = composeTypes.Services

type BuildConfig = composeTypes.BuildConfig

type LoaderOptions struct {
	ConfigPaths []string
	ProjectName string
}

type Loader struct {
	options LoaderOptions
	cached  *Project
}

type LoaderOption func(*LoaderOptions)

func WithPath(paths ...string) LoaderOption {
	return func(o *LoaderOptions) {
		o.ConfigPaths = paths
	}
}

func WithProjectName(name string) LoaderOption {
	return func(o *LoaderOptions) {
		o.ProjectName = name
	}
}

func NewLoader(opts ...LoaderOption) *Loader {
	options := LoaderOptions{}
	for _, o := range opts {
		o(&options)
	}

	// if no --project-name is provided, try to get it from the environment
	// https://docs.docker.com/compose/project-name/#set-a-project-name
	if options.ProjectName == "" {
		if envProjName, ok := os.LookupEnv("COMPOSE_PROJECT_NAME"); ok {
			options.ProjectName = envProjName
		}
	}

	return &Loader{options: options}
}

func (l *Loader) LoadProjectName(ctx context.Context) (string, bool, error) {
	if l.options.ProjectName != "" {
		return l.options.ProjectName, false, nil
	}

	project, err := l.loadProject(ctx, true)
	if err != nil {
		if errors.Is(err, types.ErrComposeFileNotFound) {
			return "", false, fmt.Errorf("no --project-name specified and %w", err)
		}
		return "", false, err
	}

	return project.Name, true, nil
}

func (l *Loader) LoadProject(ctx context.Context) (*Project, error) {
	return l.loadProject(ctx, false)
}

func (l *Loader) TargetDirectory(ctx context.Context) string {
	project, _ := l.loadProject(ctx, true)
	if project == nil {
		return ""
	}

	return project.WorkingDir
}

func (l *Loader) loadProject(ctx context.Context, suppressWarn bool) (*Project, error) {
	if l.cached != nil {
		return l.cached, nil
	}

	projOpts, err := l.newProjectOptions(suppressWarn)
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
		term.Println(string(b))
	}

	l.cached = project
	return project, nil
}

func (l *Loader) newProjectOptions(suppressWarn bool) (*cli.ProjectOptions, error) {
	// Set logrus send logs via the term package
	termLogger := logs.TermLogFormatter{Term: term.DefaultTerm}
	logrus.SetFormatter(termLogger)

	onlyComposeEnv := slices.DeleteFunc(os.Environ(), func(kv string) bool {
		return !strings.HasPrefix(kv, "COMPOSE_") // only keep COMPOSE_* variables
	})

	// Based on how docker compose setup its own project options
	// https://github.com/docker/compose/blob/1a14fcb1e6645dd92f5a4f2da00071bd59c2e887/cmd/compose/compose.go#L326-L346
	return cli.NewProjectOptions(l.options.ConfigPaths,
		// First apply os.Environment, always win
		// -- DISABLED FOR DEFANG -- cli.WithOsEnv,
		cli.WithEnv(onlyComposeEnv),
		// Load PWD/.env if present and no explicit --env-file has been set
		cli.WithEnvFiles(), // TODO: Support --env-file to be added as param to this call
		// read dot env file to populate project environment
		cli.WithDotEnv,
		// get compose file path set by COMPOSE_FILE
		cli.WithConfigFileEnv,
		// if none was selected, get default compose.yaml file from current dir or parent folder
		cli.WithDefaultConfigPath,
		// Calling the 2 functions below the 2nd time as the loaded env in first call modifies the behavior of the 2nd call:
		// .. and then, a project directory != PWD maybe has been set so let's load .env file
		cli.WithEnvFiles(), // TODO: Support --env-file to be added as param to this call
		cli.WithDotEnv,
		// eventually COMPOSE_PROFILES should have been set
		// cli.WithDefaultProfiles(c.Profiles...), TODO: Support --profile to be added as param to this call
		cli.WithName(l.options.ProjectName),
		// DEFANG SPECIFIC OPTIONS
		cli.WithDefaultProfiles("defang"), // FIXME: this overrides any COMPOSE_PROFILES env
		cli.WithDiscardEnvFile,
		cli.WithConsistency(false), // TODO: check fails if secrets are used but top-level 'secrets:' is missing
		cli.WithLoadOptions(func(o *loader.Options) {
			// As suggested by https://github.com/compose-spec/compose-go/issues/710#issuecomment-2462287043, we'll be called again once the project is loaded
			if o.Interpolate == nil {
				return
			}
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
						if inEnv && !suppressWarn {
							term.Warnf("Environment variable %q is ignored; add it to `.env` if needed", key)
						} else {
							term.Debugf("Unresolved environment variable %q", key)
						}
						return "", false
					}
					if inEnv && !suppressWarn {
						term.Warnf("Environment variable %q is ignored; add it to `.env` or it may be resolved from config during deployment", key)
					} else {
						term.Debugf("Environment variable %q was not resolved locally. It may be resolved from config during deployment", key)
					}
					// Leave unresolved variables as-is for resolution later by CD
					return "${" + key + "}", true
				})
			}
		}),
	)
}

func hasSubstitution(s, key string) bool {
	// Check in the original `templ` string if the variable uses any substitution patterns like - :- + :+ ? :?
	pattern := regexp.MustCompile(`(^|[^$])\$\{` + regexp.QuoteMeta(key) + `:?[-+?]`)
	return pattern.MatchString(s)
}

func (l *Loader) CreateProjectForDebug() (*Project, error) {
	projOpts, err := l.newProjectOptions(true)
	if err != nil {
		return nil, err
	}

	// get the project name
	if projOpts.Name == "" {
		dir, err := os.Getwd()
		if err != nil {
			return nil, err
		}

		projOpts.Name = filepath.Base(dir)
	}
	project := &Project{
		Name:         projOpts.Name,
		WorkingDir:   projOpts.WorkingDir,
		Environment:  projOpts.Environment,
		ComposeFiles: projOpts.ConfigPaths,
	}

	return project, nil
}
