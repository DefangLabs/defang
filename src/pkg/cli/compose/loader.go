package compose

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/logs"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
	"github.com/compose-spec/compose-go/v2/cli"
	"github.com/compose-spec/compose-go/v2/consts"
	"github.com/compose-spec/compose-go/v2/errdefs"
	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/template"
	composeTypes "github.com/compose-spec/compose-go/v2/types"
	"github.com/sirupsen/logrus"
	"go.yaml.in/yaml/v4"
)

type Project = composeTypes.Project

type ServiceConfig = composeTypes.ServiceConfig

type Services = composeTypes.Services

type BuildConfig = composeTypes.BuildConfig

// composeEnvFilesEnvVar lists the env file(s) to load when no explicit env
// file is set, matching `docker compose`'s COMPOSE_ENV_FILES (comma-separated).
const composeEnvFilesEnvVar = "COMPOSE_ENV_FILES"

type LoaderOptions struct {
	ConfigPaths []string
	ProjectName string
	EnvFiles    []string
	// InterpolationEnv contains values supplied by the CLI for Compose
	// interpolation. These values take precedence over values from env files.
	InterpolationEnv map[string]string
	// DefaultEnvFiles are candidate env files (names resolved against the project
	// working directory) tried when no env file was set explicitly (via EnvFiles
	// or COMPOSE_ENV_FILES). Files that don't exist are skipped, and later files
	// override earlier ones. Unlike EnvFiles, these never fail on a missing file.
	DefaultEnvFiles []string
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

// WithEnvFiles sets the env file(s) used to populate the project environment for
// interpolation, mirroring `docker compose --env-file`. When empty, the loader
// falls back to the COMPOSE_ENV_FILES environment variable, and finally to the
// default PWD/.env file (if present).
func WithEnvFiles(paths ...string) LoaderOption {
	return func(o *LoaderOptions) {
		o.EnvFiles = paths
	}
}

func WithDefaultEnvFiles(paths ...string) LoaderOption {
	return func(o *LoaderOptions) {
		o.DefaultEnvFiles = paths
	}
}

// WithInterpolationEnv adds CLI-supplied values to the Compose interpolation
// environment. These values take precedence over values from env files.
func WithInterpolationEnv(env map[string]string) LoaderOption {
	return func(o *LoaderOptions) {
		o.InterpolationEnv = env
	}
}

func NewLoader(opts ...LoaderOption) *Loader {
	options := LoaderOptions{}
	for _, o := range opts {
		o(&options)
	}
	return NewLoaderFromOptions(options)
}

func NewLoaderFromOptions(options LoaderOptions) *Loader {
	// If no --project-name is provided, try to get it from the environment
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

	project, err := l.loadProject(ctx, true) // FIXME: warnings are dropped because the project will have been cached
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

func (l *Loader) ProjectWorkingDir(ctx context.Context) (string, error) {
	project, err := l.loadProject(ctx, true) // FIXME: warnings are dropped because the project will have been cached
	if err != nil {
		return "", err
	}

	return project.WorkingDir, nil
}

// ResolveProjectWorkingDir returns the project directory without parsing or caching the Compose project.
func (l *Loader) ResolveProjectWorkingDir(context.Context) (string, error) {
	if l.cached != nil {
		return l.cached.WorkingDir, nil
	}

	projectOptions, err := l.newProjectOptions(true)
	if err != nil {
		return "", err
	}
	return projectOptions.GetWorkingDir()
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
	interpolationEnv := composeTypes.NewMapping(onlyComposeEnv)
	for name, value := range l.options.InterpolationEnv {
		interpolationEnv[name] = value
	}

	// The explicit --env-file(s) win; otherwise fall back to COMPOSE_ENV_FILES,
	// symmetric to how ConfigPaths overrides COMPOSE_FILE (cli.WithConfigFileEnv)
	// below. Reading the env var here (at load time) rather than up front is what
	// lets a value injected by the selected stack file (via LoadStackEnv) take
	// effect, since the stack env is applied before the project is loaded.
	envFiles := l.options.EnvFiles
	if len(envFiles) == 0 {
		if v := os.Getenv(composeEnvFilesEnvVar); v != "" {
			envFiles = strings.Split(v, ",")
		}
	}
	// Only when no env file was set explicitly, fall back to the default env files.
	withEnvFiles := cli.WithEnvFiles(envFiles...)
	if len(envFiles) == 0 && len(l.options.DefaultEnvFiles) > 0 {
		withEnvFiles = withDefaultEnvFiles(l.options.DefaultEnvFiles)
	}

	// Based on how docker compose setup its own project options
	// https://github.com/docker/compose/blob/1a14fcb1e6645dd92f5a4f2da00071bd59c2e887/cmd/compose/compose.go#L326-L346
	return cli.NewProjectOptions(l.options.ConfigPaths,
		// First apply os.Environment, always win
		// -- DISABLED FOR DEFANG -- cli.WithOsEnv,
		cli.WithEnv(interpolationEnv.Values()),
		// Load the explicit --env-file(s), or the convention-based/default .env files if none were set
		withEnvFiles,
		// read dot env file to populate project environment
		cli.WithDotEnv,
		// get compose file path set by COMPOSE_FILE
		cli.WithConfigFileEnv,
		// if none was selected, get default compose.yaml file from current dir or parent folder
		cli.WithDefaultConfigPath,
		// Calling the 2 functions below the 2nd time as the loaded env in first call modifies the behavior of the 2nd call:
		// .. and then, a project directory != PWD maybe has been set so let's load .env file
		withEnvFiles,
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

// withDefaultEnvFiles selects the env files from the given candidate names,
// resolved against the project working directory, so later files override
// earlier ones. Unlike explicit --env-file values, files that don't exist are
// skipped, mirroring how compose-go treats the default .env (see cli.WithEnvFiles).
func withDefaultEnvFiles(names []string) cli.ProjectOptionsFn {
	return func(o *cli.ProjectOptions) error {
		if v, ok := os.LookupEnv(consts.ComposeDisableDefaultEnvFile); ok {
			if disabled, err := strconv.ParseBool(v); err != nil {
				return err
			} else if disabled {
				return nil
			}
		}

		wd, err := o.GetWorkingDir()
		if err != nil {
			return err
		}
		var envFiles []string
		for _, name := range slices.Compact(slices.Clone(names)) { // dedupe e.g. a stack named after its provider
			path := filepath.Join(wd, name)
			s, err := os.Stat(path)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					continue // missing default env files are optional
				}
				return fmt.Errorf("default env file: %w", err)
			}
			if !s.IsDir() {
				envFiles = append(envFiles, path)
			}
		}
		o.EnvFiles = envFiles
		return nil
	}
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
