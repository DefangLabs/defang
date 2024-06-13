package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/compose-spec/compose-go/v2/loader"
	compose "github.com/compose-spec/compose-go/v2/types"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

type ComposeLoader struct {
	ComposeFilePath string
}

// LoadCompose will determine the project name using behavior consistent with docker-compose
// https://docs.docker.com/compose/project-name/
//  1. -p flag (currently not implemented)
//  2. COMPOSE_PROJECT_NAME environment variable
//  3. top level 'name' attribute in the Compose file, ** handled by compose-go **
//  4. The base name of the project directory:
//     a. directory containing your Compose file.
//     b. the base name of the first Compose file if you specify multiple Compose files in the command line with the -f flag. ** TODO: support multiple Compose files **
//  5. The base name of the current directory if no Compose file is specified. ** Irrelevant for us **
func (c ComposeLoader) LoadCompose() (*compose.Project, error) {
	filePath, err := getComposeFilePath(c.ComposeFilePath)
	if err != nil {
		return nil, err
	}

	term.Debug("Loading compose file", filePath)

	// Compose-go uses the logrus logger, so we need to configure it to be more like our own logger
	logrus.SetFormatter(&logrus.TextFormatter{DisableTimestamp: true, DisableColors: !term.StderrCanColor(), DisableLevelTruncation: true})

	loadCfg := compose.ConfigDetails{
		WorkingDir:  filepath.Dir(filePath),
		ConfigFiles: []compose.ConfigFile{{Filename: filePath}},
		Environment: map[string]string{}, // TODO: support environment variables?
	}

	overrideProjName := false
	//  2. COMPOSE_PROJECT_NAME environment variable
	projName := os.Getenv("COMPOSE_PROJECT_NAME")
	if projName != "" {
		term.Debugf("using COMPOSE_PROJECT_NAME environment variable %q as project name", projName)
		overrideProjName = true
	} else {
		// 4a. Use directory containing your Compose file as the default project name
		fullPath, err := filepath.Abs(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to get parent directory name of compose file %q: %w", filePath, err)
		}
		projName = filepath.Base(filepath.Dir(fullPath))
		term.Debugf("using directory name %q of compose file %q as default project name", projName, filePath)
		// TODO: 4b. the base name of the first Compose file if you specify multiple Compose files in the command line with the -f flag.
		// TODO: Support multiple Compose files
	}

	// Normalize project name
	if projName != ProjectNameSafe(projName) {
		term.Warnf("project name %q does not conform to docker-compose naming rules; using %q instead", projName, ProjectNameSafe(projName))
		projName = ProjectNameSafe(projName)
	}

	loadOpts := []func(*loader.Options){
		loader.WithDiscardEnvFiles,
		loader.WithProfiles([]string{"defang"}), // TODO: add stage-specific profiles once we have them
		func(o *loader.Options) {
			o.SkipConsistencyCheck = true // TODO: check fails if secrets are used but top-level 'secrets:' is missing
			o.SetProjectName(projName, overrideProjName)
		},
	}

	project, err := loader.Load(loadCfg, loadOpts...)
	if err != nil {
		return nil, err
	}

	// Hack: Fill in the missing environment variables that were stripped by the normalization process
	skipNormalizationOpts := append(loadOpts, func(o *loader.Options) {
		o.SkipNormalization = true // Normalization strips environment variables keys that does not have an value
	})

	// Disable logrus output to prevent double warnings from compose-go
	currentOutput := logrus.StandardLogger().Out
	logrus.SetOutput(io.Discard)
	rawProj, err := loader.Load(loadCfg, skipNormalizationOpts...)
	logrus.SetOutput(currentOutput)
	if err != nil {
		return nil, err // there's no good reason this should fail, since we've already loaded the project
	}

	// TODO: file a PR to compose-go to add option to keep unset environment variables
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

// Project names must contain only lowercase letters, decimal digits, dashes, and underscores, and must begin with a lowercase letter or decimal digit.
// https://github.com/compose-spec/compose-spec/blob/master/spec.md#the-compose-application-model
func ProjectNameSafe(name string) string {
	var result strings.Builder
	result.Grow(len(name))
	for i, c := range name {
		// Convert all non-alphanumeric and non '-' characters to underscores
		if !isAlphaNumeric(c) && c != '-' {
			c = '_'
		}
		// First character must be a letter or number
		if i == 0 && !isAlphaNumeric(c) {
			result.WriteRune('0')
		}

		c = unicode.ToLower(c)
		result.WriteRune(c)
	}
	return result.String()
}

func isAlphaNumeric(c rune) bool {
	return ('a' <= c && c <= 'z') ||
		('A' <= c && c <= 'Z') ||
		('0' <= c && c <= '9')
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
