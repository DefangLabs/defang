package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/compose-spec/compose-go/v2/loader"
	compose "github.com/compose-spec/compose-go/v2/types"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

type ComposeLoader struct {
	ComposeFilePath string
}

func (c ComposeLoader) LoadWithDefaultProjectName(name string) (*compose.Project, error) {
	return loadCompose(c.ComposeFilePath, name, false) // use tenantID as fallback for project name
}

func (c ComposeLoader) LoadWithProjectName(name string) (*compose.Project, error) {
	return loadCompose(c.ComposeFilePath, name, true)
}

func loadCompose(filePath string, projectName string, overrideProjectName bool) (*compose.Project, error) {
	filePath, err := getComposeFilePath(filePath)
	if err != nil {
		return nil, err
	}

	term.Debug(" - Loading compose file", filePath)

	// Compose-go uses the logrus logger, so we need to configure it to be more like our own logger
	logrus.SetFormatter(&logrus.TextFormatter{DisableTimestamp: true, DisableColors: !term.CanColorErr, DisableLevelTruncation: true})

	loadCfg := compose.ConfigDetails{
		WorkingDir:  filepath.Dir(filePath),
		ConfigFiles: []compose.ConfigFile{{Filename: filePath}},
		Environment: map[string]string{}, // TODO: support environment variables?
	}

	skipNormalizationOpts := []func(*loader.Options){
		loader.WithDiscardEnvFiles,
		func(o *loader.Options) {
			o.SkipConsistencyCheck = true
			o.SetProjectName(strings.ToLower(projectName), overrideProjectName)
			o.SkipNormalization = true // Normalization strips environment variables keys that does not have an value
		},
	}

	// Disable logrus output to prevent double warnings from compose-go
	currentOutput := logrus.StandardLogger().Out
	logrus.SetOutput(io.Discard)
	rawProj, err := loader.Load(loadCfg, skipNormalizationOpts...)
	logrus.SetOutput(currentOutput)
	if err != nil {
		return nil, err
	}

	loadOpts := []func(*loader.Options){
		loader.WithDiscardEnvFiles,
		func(o *loader.Options) {
			o.SkipConsistencyCheck = true // TODO: check fails if secrets are used but top-level 'secrets:' is missing
			o.SetProjectName(strings.ToLower(projectName), overrideProjectName)
		},
	}

	project, err := loader.Load(loadCfg, loadOpts...)
	if err != nil {
		return nil, err
	}

	// Hack: Fill in the missing environment variables that were stripped by the normalization process
	// TODO: file a PR to compose-go to add option to keep unset environment variables
	for i, service := range rawProj.Services {
		for key, value := range service.Environment {
			svc := project.Services[i]
			if svc.Environment[key] == nil {
				if svc.Environment == nil {
					svc.Environment = make(map[string]*string)
				}
				svc.Environment[key] = value
				project.Services[i] = svc
			}
		}
	}

	if term.DoDebug {
		b, _ := yaml.Marshal(project)
		fmt.Println(string(b))
	}
	return project, nil
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
	term.Debug(" - Looking for compose file - searching for", searchPattern)
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
